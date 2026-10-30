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
const cacheFormat = 2

// releasesPerPage is how many releases one lookup reads, the newest ones.
const releasesPerPage = 30

// getJSON reads one API answer into v. A limit answer is a *httpkit.RateLimitError, any other non-200 a
// *httpkit.StatusError, a body over limit safefs.ErrTooLarge, and a transport failure the client's own error.
func getJSON(ctx context.Context, hc *http.Client, rawURL string, limit int64, v any) error {
	ctx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	DefaultAuth.Apply(req)
	body, err := httpkit.DoCapped(httpkit.WithRedirect(cmp.Or(hc, http.DefaultClient), httpkit.NoDowngrade), req, limit, Observe)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("read GitHub's response: %w", err)
	}
	return nil
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

// repoURL is the API URL of owner/repo, or ErrInvalidRepo before anything is spliced into it.
func (c *Client) repoURL(owner, repo string) (string, error) {
	if !ValidRepo(owner + "/" + repo) {
		return "", fmt.Errorf("%w: %q", ErrInvalidRepo, owner+"/"+repo)
	}
	return cmp.Or(c.APIBase, apiBase) + "/repos/" + owner + "/" + repo, nil
}

func releasesName(owner, repo string) string {
	return Key(owner, repo, "", "") + "-releases-v2.json"
}

// CachedReleases lists owner/repo's releases from the stored answer however old it is, never asking GitHub. all is
// whether the list holds every release the repository has, which a full page may not; ok needs the current cache
// format.
func (c *Client) CachedReleases(owner, repo string) (releases []Release, all, ok bool) {
	e, ok := diskcache.Peek[[]Release](c.cache(), releasesName(owner, repo))
	return e.Value, len(e.Value) < releasesPerPage, ok && e.Stamp == strconv.Itoa(cacheFormat)
}

// Releases lists owner/repo's newest releases, as GitHub orders them. An answer younger than an hour comes from the
// cache and a stale one stands in when the lookup fails, rate limit included. An owner/repo that is not a GitHub name
// is ErrInvalidRepo, with no request.
func (c *Client) Releases(ctx context.Context, owner, repo string) ([]Release, error) {
	base, err := c.repoURL(owner, repo)
	if err != nil {
		return nil, err
	}
	return diskcache.CachedStamped(c.cache(), releasesName(owner, repo), strconv.Itoa(cacheFormat), cacheTTL, func() ([]Release, error) {
		return FetchReleases(ctx, c.HTTP, base+"/releases?per_page="+strconv.Itoa(releasesPerPage))
	})
}

// ReleaseNotes returns the markdown body of owner/repo's release tagged tag. A published release never changes
// its notes, so a cached answer is final and never refetched; a failed lookup is not cached. An invalid owner/repo
// is ErrInvalidRepo, and a tag that is empty or would move the URL's path is ErrNoRelease, both with no request.
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
		Body string `json:"body"`
	}
	if err := getJSON(ctx, c.HTTP, base+"/releases/tags/"+url.PathEscape(tag), maxNotes, &rel); err != nil {
		return "", err
	}
	notes := strings.TrimSpace(rel.Body)
	_ = diskcache.Put(c.cache(), name, notes)
	return notes, nil
}
