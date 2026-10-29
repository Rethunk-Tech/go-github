# Changelog

Notable changes to `go-github`. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- `Release`, `Asset`, `FetchReleases` and `Client` with a disk-cached, stale-on-failure `Releases`, `CachedReleases`,
  `ReleaseNotes` and `ReleasesBetween`.
- `Select` with a `keep` predicate, `Installable`, `Shape` and `Key` for choosing and recognising release assets.
- `ZipNames`, a remote zip's file list over ranged requests.
- `Observe`, `RateLimited`, `CurrentRate` and `Rate` for GitHub's quota.
- `Auth`, `DefaultAuth` and `GHCLI` for an opt-in `gh` login; `ValidRepo` and `ErrInvalidRepo` for names.

[Unreleased]: https://github.com/Rethunk-Tech/go-github/commits/main
