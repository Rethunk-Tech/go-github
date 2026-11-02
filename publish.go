package github

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/Rethunk-Tech/go-httpkit"
)

// ErrNoToken is the answer to publishing without a token: DefaultAuth has no Run, or its login failed.
var ErrNoToken = errors.New("publishing needs a GitHub token: set DefaultAuth.Run (GHCLI uses the gh login)")

// NewRelease describes a release to create.
type NewRelease struct {
	Tag, Name, Body string
	// Target is the branch or commit the tag is created from when it does not exist yet; "" means the default branch.
	Target     string
	Draft      bool
	Prerelease bool
}

const maxReply = 1 << 20

// send runs req through DefaultAuth and reads a capped answer. A token is required. A non-2xx answer is an error
// that wraps *httpkit.StatusError and carries GitHub's message, so errors.Is(err, httpkit.ErrNotFound) holds for a
// 404. The token goes only to https GitHub hosts (Auth.Apply), so a redirect or override elsewhere sends none.
func (c *Client) send(req *http.Request, v any) error {
	if !DefaultAuth.LoggedIn(req.Context()) {
		return ErrNoToken
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	DefaultAuth.Apply(req)
	resp, err := httpkit.WithRedirect(cmp.Or(c.HTTP, http.DefaultClient), httpkit.NoDowngrade).Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if err := Observe(resp); err != nil {
		return err
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxReply))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		var msg struct {
			Message string `json:"message"`
		}
		_ = json.Unmarshal(body, &msg)
		se := &httpkit.StatusError{Code: resp.StatusCode, Status: resp.Status}
		if msg.Message == "" {
			return se
		}
		return fmt.Errorf("%w: %s", se, msg.Message)
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("read GitHub's response: %w", err)
	}
	return nil
}

// CreateRelease creates a release in owner/repo and returns it, with the ID and UploadURL that UploadAsset needs. A
// tag that is already released is a 422 *httpkit.StatusError; ReleaseByTag tells that beforehand. A missing token is
// ErrNoToken and an invalid owner/repo ErrInvalidRepo, both with no request.
func (c *Client) CreateRelease(ctx context.Context, owner, repo string, r NewRelease) (Release, error) {
	base, err := c.repoURL(owner, repo)
	if err != nil {
		return Release{}, err
	}
	payload := map[string]any{"tag_name": r.Tag, "name": r.Name, "body": r.Body, "draft": r.Draft, "prerelease": r.Prerelease}
	if r.Target != "" {
		payload["target_commitish"] = r.Target
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return Release{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/releases", bytes.NewReader(b))
	if err != nil {
		return Release{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	var rel Release
	if err := c.send(req, &rel); err != nil {
		return Release{}, err
	}
	return rel, nil
}

// ReleaseByTag looks up owner/repo's release tagged tag, drafts included when the token may see them. found is false
// with a nil error when there is none. It never reads the cache, so it is current. A tag that is empty or would move
// the URL's path is ErrNoRelease, with no request.
func (c *Client) ReleaseByTag(ctx context.Context, owner, repo, tag string) (rel Release, found bool, err error) {
	base, err := c.repoURL(owner, repo)
	if err != nil {
		return Release{}, false, err
	}
	if tag == "" || tag == "." || tag == ".." {
		return Release{}, false, ErrNoRelease
	}
	ctx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/releases/tags/"+url.PathEscape(tag), nil)
	if err != nil {
		return Release{}, false, err
	}
	if err := c.send(req, &rel); err != nil {
		if errors.Is(err, httpkit.ErrNotFound) {
			return Release{}, false, nil
		}
		return Release{}, false, err
	}
	return rel, true, nil
}

// UploadAsset streams the file at path to rel's upload URL as an asset named after the file, with Content-Length, and
// returns it. contentType "" means application/octet-stream. rel comes from CreateRelease or ReleaseByTag. The token
// is attached only when the upload URL is an https GitHub host.
func (c *Client) UploadAsset(ctx context.Context, rel Release, path, contentType string) (Asset, error) {
	// The template is "https://uploads.github.com/repos/o/r/releases/1/assets{?name,label}".
	raw, _, _ := strings.Cut(rel.UploadURL, "{")
	u, err := url.Parse(raw)
	if raw == "" || err != nil {
		return Asset{}, fmt.Errorf("release %q has no usable upload URL", rel.Tag)
	}
	f, err := os.Open(path) // #nosec G304 -- the caller names the file to publish
	if err != nil {
		return Asset{}, err
	}
	defer func() { _ = f.Close() }()
	st, err := f.Stat()
	if err != nil {
		return Asset{}, err
	}
	q := u.Query()
	q.Set("name", filepath.Base(path))
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u.String(), f)
	if err != nil {
		return Asset{}, err
	}
	req.ContentLength = st.Size()
	req.Header.Set("Content-Type", cmp.Or(contentType, "application/octet-stream"))
	var a Asset
	if err := c.send(req, &a); err != nil {
		return Asset{}, err
	}
	return a, nil
}
