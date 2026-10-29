package github

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"regexp"
	"strings"

	"github.com/Rethunk-Tech/go-semverish"
)

// Release is one entry of a repository's releases list.
type Release struct {
	Tag        string  `json:"tag_name"`
	Draft      bool    `json:"draft"`
	Prerelease bool    `json:"prerelease"`
	Body       string  `json:"body,omitempty"`
	Published  string  `json:"published_at,omitempty"`
	Assets     []Asset `json:"assets"`
}

// Asset is one downloadable file of a release.
type Asset struct {
	ID          int64  `json:"id"`
	Name        string `json:"name"`
	ContentType string `json:"content_type"`
	Size        int64  `json:"size"`
	URL         string `json:"browser_download_url"`
	Downloads   int    `json:"download_count"`
	// Digest is "sha256:<hex>" when GitHub computed one for the asset.
	Digest string `json:"digest,omitempty"`
}

// ErrNoRelease and ErrNoAsset say why Select found nothing to install.
var (
	ErrNoRelease = errors.New("no matching release")
	ErrNoAsset   = errors.New("the release has no matching asset")
)

var nonKey = regexp.MustCompile(`[^a-z0-9.]+`)

// maxSlug bounds the readable part of a key, keeping a folder named by it under a file system's name limit.
const maxSlug = 100

// Key is a stable, file-name-safe id of a release asset: github-<owner>-<repo>-<tag>-<asset>, lower-cased with every
// other character run folded to a dash, then a hash of the exact names, since the folding alone maps a-b/c and a/b-c
// to one key. Callers store it, so its bytes are a contract and never change.
func Key(owner, repo, tag, asset string) string {
	slug := nonKey.ReplaceAllString(strings.ToLower(strings.Join([]string{owner, repo, tag, asset}, "-")), "-")
	slug = strings.Trim(slug[:min(len(slug), maxSlug)], "-")
	sum := sha256.Sum256([]byte(strings.ToLower(owner+"/"+repo) + "@" + tag + "/" + asset))
	return "github-" + slug + "-" + hex.EncodeToString(sum[:6])
}

// Select picks the newest non-draft release whose tag is version (any when version is empty) and returns it with its
// assets that keep accepts (every asset when keep is nil). A prerelease is picked only when prerelease is true or its
// tag is named. A release with no kept asset is ErrNoAsset, not a reason to look at an older one.
func Select(all []Release, version string, prerelease bool, keep func(Asset) bool) (Release, []Asset, error) {
	for _, r := range all {
		if r.Draft || (r.Prerelease && !prerelease && version == "") {
			continue
		}
		if version != "" {
			if c, ok := semverish.Compare(r.Tag, version); !ok || c != 0 {
				continue
			}
		}
		var assets []Asset
		for _, a := range r.Assets {
			if keep == nil || keep(a) {
				assets = append(assets, a)
			}
		}
		if len(assets) == 0 {
			return r, nil, ErrNoAsset
		}
		return r, assets, nil
	}
	return Release{}, nil, ErrNoRelease
}
