<h1 align="center">go-github</h1>

<div align="center">

[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)
[![Go](https://img.shields.io/badge/go-1.27.3+-00ADD8?logo=go&logoColor=white)](go.mod)

</div>

---

A GitHub releases client for programs that install from release assets: a cached release list, version and asset selection, remote zip listing and the user's gh login. Package `github`, module `github.com/Rethunk-Tech/go-github`.

For installers, launchers and updaters that ask about many repositories under GitHub's 60-an-hour limit without a
token (5,000 with one). Answers are cached on disk, a stale list stands in when GitHub fails, and nothing runs `gh`
unless you set `DefaultAuth.Run`. `Key` and the cache file names are stable across releases of this module; leave
`Client.CacheDir` empty to disable caching.

## Quick start

```bash
go get github.com/Rethunk-Tech/go-github
```

A worked example and the contracts: [HUMANS.md](HUMANS.md).

## Features

- Cached release lists (one hour, stale on failure) and release notes (kept forever).
- `Select` a release by version and keep the assets you want; `Installable` drops source, checksum and other-OS builds.
- `Key` and `Shape` recognise one download across releases.
- `ZipNames` reads a remote zip's file list with a few ranged requests.
- `Observe` and `CurrentRate` track the quota; `RateLimited` names the wait.
- Opt-in `gh` login for the higher limit, sent only over https to GitHub.

## Documentation

| Doc | Audience |
| --- | --- |
| [HUMANS.md](HUMANS.md) | Running and using the package |
| [AGENTS.md](AGENTS.md) | Internals, invariants, and why |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Process — commits, tests, docs |
| [SECURITY.md](SECURITY.md) | Vulnerability disclosure |
| [CHANGELOG.md](CHANGELOG.md) | Release notes |

## License

MIT — see [LICENSE](LICENSE). © 2026 Rethunk.Tech, LLC.
