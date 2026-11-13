package github

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Rethunk-Tech/go-httpkit"
	"github.com/Rethunk-Tech/go-safefs/diskcache"
)

const (
	apiBase    = "https://api.github.com"
	cacheTTL   = time.Hour
	apiTimeout = 8 * time.Second
	maxList    = 8 << 20
	maxNotes   = 1 << 20
)

// cacheFormat stamps the release cache and is bumped when Release gains a field an older file lacks: that file is
// fetched again however young, still stands in when the lookup fails, and is never read for the missing field.
const cacheFormat = 3

// releasesPerPage is one page of a lookup; maxReleasePages bounds how many pages follow each other.
const (
	releasesPerPage = 30
	maxReleasePages = 10
)

// getJSON reads one API answer into v. A limit answer is a *httpkit.RateLimitError, any other non-200 a
// *httpkit.StatusError, a body over limit safefs.ErrTooLarge, and a transport failure the client's own error.
func getJSON(ctx context.Context, hc *http.Client, rawURL string, limit int64, v any) error {
	body, _, err := getBody(ctx, hc, rawURL, "application/vnd.github+json", limit)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("read GitHub's response: %w", err)
	}
	return nil
}

// getBody reads one API answer's body, asking for the media type accept, and returns the Link header it came with.
func getBody(ctx context.Context, hc *http.Client, rawURL, accept string, limit int64) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("Accept", accept)
	DefaultAuth.Apply(req)
	var link string
	body, err := httpkit.DoCapped(httpkit.WithRedirect(cmp.Or(hc, http.DefaultClient), httpkit.NoDowngrade), req, limit, func(resp *http.Response) error {
		link = resp.Header.Get("Link")
		return Observe(resp)
	})
	return body, link, err
}

// nextLink is the rel="next" target of a Link header, or "" when there is none.
func nextLink(header string) string {
	for part := range strings.SplitSeq(header, ",") {
		target, params, ok := strings.Cut(part, ";")
		if ok && strings.Contains(params, `rel="next"`) {
			return strings.Trim(strings.TrimSpace(target), "<>")
		}
	}
	return ""
}

// sameOrigin reports whether b is on a's scheme and host, so a Link header cannot send the token elsewhere.
func sameOrigin(a, b string) bool {
	ua, ea := url.Parse(a)
	ub, eb := url.Parse(b)
	return ea == nil && eb == nil && ua.Scheme == ub.Scheme && ua.Host == ub.Host
}

// FetchReleases reads one releases list from url (any /repos/{o}/{r}/releases page). A limit answer is a
// *httpkit.RateLimitError; any other non-200 a *httpkit.StatusError; a body over 8 MiB safefs.ErrTooLarge. Redirects
// to plain http are refused.
func FetchReleases(ctx context.Context, hc *http.Client, rawURL string) ([]Release, error) {
	var all []Release
	if err := getJSON(ctx, hc, rawURL, maxList, &all); err != nil {
		return nil, err
	}
	return all, nil
}

// Client reads releases through an on-disk cache.
type Client struct {
	// HTTP is the client every request goes through; nil means http.DefaultClient.
	HTTP *http.Client
	// CacheDir holds the cache files; "" disables the cache, so every call fetches.
	CacheDir string
	// APIBase overrides https://api.github.com, for tests and GitHub Enterprise.
	APIBase string
	// Now is the clock; nil means time.Now().UTC().
	Now func() time.Time
}

func (c *Client) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now().UTC()
}

func (c *Client) cache() *diskcache.Cache {
	if c.CacheDir == "" {
		return nil
	}
	return &diskcache.Cache{Dir: c.CacheDir, Now: c.now}
}

func (c *Client) apiBase() string { return cmp.Or(c.APIBase, apiBase) }

// repoURL is the API URL of owner/repo, or ErrInvalidRepo before anything is spliced into it.
func (c *Client) repoURL(owner, repo string) (string, error) {
	if !ValidRepo(owner + "/" + repo) {
		return "", fmt.Errorf("%w: %q", ErrInvalidRepo, owner+"/"+repo)
	}
	return c.apiBase() + "/repos/" + owner + "/" + repo, nil
}

func releasesName(owner, repo string) string {
	return Key(owner, repo, "", "") + "-releases-v2.json"
}

// CachedReleases lists owner/repo's releases from the stored answer however old it is, never asking GitHub. all is
// whether the list holds every release the repository has, which a full page may not; ok needs the current cache
// format.
func (c *Client) CachedReleases(owner, repo string) (releases []Release, all, ok bool) {
	e, ok := diskcache.Peek[[]Release](c.cache(), releasesName(owner, repo))
	return e.Value, len(e.Value) < releasesPerPage*maxReleasePages, ok && e.Stamp == strconv.Itoa(cacheFormat)
}

// Releases lists owner/repo's newest releases (up to 300, following GitHub's pagination), as GitHub orders them. An answer younger than an hour comes from the
// cache and a stale one stands in when the lookup fails, rate limit included. An owner/repo that is not a GitHub name
// is ErrInvalidRepo, with no request.
func (c *Client) Releases(ctx context.Context, owner, repo string) ([]Release, error) {
	base, err := c.repoURL(owner, repo)
	if err != nil {
		return nil, err
	}
	return diskcache.CachedStamped(c.cache(), releasesName(owner, repo), strconv.Itoa(cacheFormat), cacheTTL, func() ([]Release, error) {
		var all []Release
		next := base + "/releases?per_page=" + strconv.Itoa(releasesPerPage)
		for range maxReleasePages {
			body, link, err := getBody(ctx, c.HTTP, next, "application/vnd.github+json", maxList)
			if err != nil {
				return nil, err
			}
			var page []Release
			if err := json.Unmarshal(body, &page); err != nil {
				return nil, fmt.Errorf("read GitHub's response: %w", err)
			}
			all = append(all, page...)
			follow := nextLink(link)
			if follow == "" || !sameOrigin(base, follow) {
				break
			}
			next = follow
		}
		return all, nil
	})
}

// ReleaseNotes returns the markdown body of owner/repo's release tagged tag. A published release never changes
// its notes, so a cached answer is final and never refetched; a draft or a failed lookup is not cached. An invalid
// owner/repo is ErrInvalidRepo, and a tag that is empty or would move the URL's path is ErrNoRelease, both with no
// request.
func (c *Client) ReleaseNotes(ctx context.Context, owner, repo, tag string) (string, error) {
	base, err := c.repoURL(owner, repo)
	if err != nil {
		return "", err
	}
	if tag == "" || tag == "." || tag == ".." {
		return "", ErrNoRelease
	}
	name := Key(owner, repo, tag, "") + "-notes-v2.json"
	if notes, ok := diskcache.Peek[string](c.cache(), name); ok {
		return notes.Value, nil
	}
	var rel struct {
		Body  string `json:"body"`
		Draft bool   `json:"draft"`
	}
	if err := getJSON(ctx, c.HTTP, base+"/releases/tags/"+url.PathEscape(tag), maxNotes, &rel); err != nil {
		return "", err
	}
	notes := strings.TrimSpace(rel.Body)
	if !rel.Draft {
		_ = diskcache.Put(c.cache(), name, notes)
	}
	return notes, nil
}
