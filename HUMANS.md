# HUMANS.md

Run and use `go-github`. Internals: [AGENTS.md](AGENTS.md).

## Quick start

```bash
go get github.com/Rethunk-Tech/go-github
```

```go
package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/Rethunk-Tech/go-github"
)

func main() {
	github.DefaultAuth.Run = github.GHCLI // optional: the 5,000-an-hour limit when gh is logged in

	dir, _ := os.UserCacheDir()
	c := &github.Client{CacheDir: filepath.Join(dir, "myapp")}
	all, err := c.Releases(context.Background(), "cli", "cli")
	if err != nil {
		fmt.Println(err)
		return
	}
	keep := func(a github.Asset) bool { return strings.HasSuffix(a.Name, ".tar.gz") }
	rel, assets, err := github.Select(all, "", false, keep)
	if err != nil {
		fmt.Println(err)
		return
	}
	for _, a := range github.Installable(assets, runtime.GOOS) {
		fmt.Println(rel.Tag, a.Name, github.Key("cli", "cli", rel.Tag, a.Name))
	}
}
```

## Contracts

| Behaviour | Contract |
| --- | --- |
| `Client.Releases` | Cached an hour; a failed fetch, the limit included, serves any stored list; `ErrInvalidRepo` before any request |
| `Client.CacheDir` | `""` disables the cache; files are `<Key>-releases-v2.json` and `<Key>-notes-v2.json` |
| `Client.CachedReleases` | Never fetches; `ok` needs the current cache format |
| `Client.ReleaseNotes` | A hit is final, a failure is not cached; the tag is path-escaped |
| `FetchReleases` | Limit answer is `*httpkit.RateLimitError`, other non-200 `*httpkit.StatusError`, body over 8 MiB `safefs.ErrTooLarge` |
| `Select` | Never a draft; a prerelease only when asked or named; a release with no kept asset is `ErrNoAsset`; `keep` nil keeps all |
| `Installable` | Each filter applies only when it leaves something |
| `Key`, `Shape` | Byte-stable: callers store them |
| `ReleasesBetween` | GitHub's order, drafts and non-version tags skipped, at most `MaxChangelog` |
| `ZipNames` | Reads the central directory only; non-zip or unknown size is an error |
| `Observe` | Records the quota, then returns `RateLimited`; fits `httpkit.DoCapped` and a download's `Check` |
| `DefaultAuth` | Sends no token until `Run` is set; token only to https `github.com` and `api.github.com`; never in an error |

## Requirements

Go 1.27.3 or newer, plus `lefthook` and `gitleaks` on PATH if you want the git hooks.

```bash
lefthook install
```

That is per clone; git does not commit hooks. Pre-commit scans staged changes for secrets, and
pre-push runs `gate`.

## Verify

```bash
gate
```
