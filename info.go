package github

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
)

const (
	maxReadme  = 8 << 20
	maxInfo    = 1 << 20
	maxLicense = 1 << 20
)

// Readme returns the raw markdown of owner/repo's README. A repository with none is an error matching
// httpkit.ErrNotFound; a limit answer is a *httpkit.RateLimitError. Nothing is cached. An invalid owner/repo is
// ErrInvalidRepo, with no request.
func (c *Client) Readme(ctx context.Context, owner, repo string) (string, error) {
	base, err := c.repoURL(owner, repo)
	if err != nil {
		return "", err
	}
	body, _, err := getBody(ctx, c.HTTP, base+"/readme", "application/vnd.github.raw", maxReadme)
	return string(body), err
}

// Archived reports whether GitHub marks owner/repo archived. A missing repository is an error matching
// httpkit.ErrNotFound. Nothing is cached.
func (c *Client) Archived(ctx context.Context, owner, repo string) (bool, error) {
	base, err := c.repoURL(owner, repo)
	if err != nil {
		return false, err
	}
	var info struct {
		Archived bool `json:"archived"`
	}
	if err := getJSON(ctx, c.HTTP, base, maxInfo, &info); err != nil {
		return false, err
	}
	return info.Archived, nil
}

// ErrInvalidLicense is returned for a license key that is not one of GitHub's lower-case SPDX-style keys.
var ErrInvalidLicense = errors.New("not a GitHub license key")

var licenseKey = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)

// LicenseText returns the full text of the license GitHub lists under key (for example "mpl-2.0"), with no holder or
// year filled in. An unknown key is an error matching httpkit.ErrNotFound; a key that is not a license key is
// ErrInvalidLicense, with no request. Nothing is cached.
func (c *Client) LicenseText(ctx context.Context, key string) (string, error) {
	if !licenseKey.MatchString(key) || key == "." || key == ".." {
		return "", fmt.Errorf("%w: %q", ErrInvalidLicense, key)
	}
	var lic struct {
		Body string `json:"body"`
	}
	if err := getJSON(ctx, c.HTTP, c.apiBase()+"/licenses/"+url.PathEscape(key), maxLicense, &lic); err != nil {
		return "", err
	}
	if lic.Body == "" {
		return "", fmt.Errorf("GitHub's license %q has no text", key)
	}
	return lic.Body, nil
}
