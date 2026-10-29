package github

import (
	"context"

	"github.com/Rethunk-Tech/go-semverish"
)

// MaxChangelog bounds how many releases ReleasesBetween returns.
const MaxChangelog = 10

// ReleasesBetween returns owner/repo's published releases strictly newer than installed and not newer than latest
// (any when latest is empty), in GitHub's order (newest published first), at most MaxChangelog. A tag that is not a
// version is skipped. The order is publish order, not version order, so a backport published later still shows. It
// reads the cached releases list.
func (c *Client) ReleasesBetween(ctx context.Context, owner, repo, installed, latest string) ([]Release, error) {
	all, err := c.Releases(ctx, owner, repo)
	if err != nil {
		return nil, err
	}
	var out []Release
	for _, r := range all {
		if r.Draft || len(out) == MaxChangelog {
			continue
		}
		if cmpInstalled, ok := semverish.Compare(r.Tag, installed); !ok || cmpInstalled <= 0 {
			continue
		}
		if cmpLatest, ok := semverish.Compare(r.Tag, latest); latest != "" && ok && cmpLatest > 0 {
			continue
		}
		out = append(out, r)
	}
	return out, nil
}
