# AGENTS.md

Package `github` lists a repository's GitHub releases through an on-disk cache, selects a release and its assets, and
lends requests a token. It builds on go-httpkit, go-safefs, go-semverish and go-desktopenv/nowindow, and imports no
program that uses it.

## Gate

`gate` is the command. Each override in `.gate.toml` is there because a runner
cannot resolve that check from PATH.

## Shape

- `Key` and `Shape` are stored by callers (folder names, remembered choices). Their bytes are a contract: the golden
  tables in the tests must never be edited to fit a change.
- Cache files are `<Key>-releases-v2.json` and `<Key>-notes-v2.json`, stamped with `cacheFormat`. A new `Release`
  field bumps the stamp so older files are fetched again but still stand in when a fetch fails. An unversioned or
  value-less file reads as absent.
- `getJSON` is the single path for API reads, over `httpkit.DoCapped` with `Observe` as the check: `Observe` sees a
  403 or 429 before the status test, so a limit answer is a `*httpkit.RateLimitError` and the quota is always recorded.
  Redirects to plain http are refused (`httpkit.NoDowngrade`).
- `owner/repo` reaches a URL only through `ValidRepo`; a tag only through `url.PathEscape`, with `.`, `..` and empty
  refused, since an escaped `..` still moves the path.
- `DefaultAuth` and `CurrentRate` are process-wide because GitHub's quota belongs to the token or IP address, not to a
  `Client`. `DefaultAuth` runs nothing until `Run` is set: starting `gh` on a first request would be a surprise.
- `Select` takes a `keep` predicate and imports no archive library, so a consumer that keeps `.exe` assets does not
  link archive code.
- `ReleasesBetween` keeps GitHub's order and never sorts by version, so a backport published later still shows.
- A test touching `CurrentRate` is not parallel.

## Files

| File | Holds |
| --- | --- |
| `doc.go` | Package comment |
| `github.go` | `Release`, `Asset`, `Key`, `Select`, `ErrNoRelease`, `ErrNoAsset` |
| `api.go` | `getJSON`, `FetchReleases`, `Client`, its cache and `ReleaseNotes` |
| `changelog.go` | `ReleasesBetween` |
| `pick.go` | `Installable`, `Shape` |
| `zip.go` | `ZipNames` over ranged requests |
| `rate.go` | `Rate`, `CurrentRate`, `RateLimited`, `Observe` |
| `publish.go` | `NewRelease`, `CreateRelease`, `UploadAsset`, `ReleaseByTag`, `ErrNoToken` |
| `auth.go` | `Auth`, `DefaultAuth`, `GHCLI`, `IsGitHubHost` |
| `repo.go` | `ValidRepo`, `ErrInvalidRepo` |
