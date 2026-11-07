# Changelog

Notable changes to `go-github`. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and versions follow
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.3.0] - 2026-11-07

### Added

- `Client.Readme`, `Archived` and `LicenseText` read a repository's README, its archived flag and a license's text through the same auth, redirect and rate-limit handling as releases; `Asset.VerifyDigest` checks a downloaded file against GitHub's asset digest.

### Fixed

- `ZipNames` refuses a redirect from https to plain http, as the API reads do.

## [0.2.0] - 2026-11-02

### Added

- `Client.CreateRelease`, `UploadAsset` and `ReleaseByTag` publish a release, attach a file to it and tell whether a tag is already released; `Release` gains `ID`, `HTMLURL` and `UploadURL`, so cached release lists are fetched again once.

## [0.1.1] - 2026-10-30

### Changed

- Requires go-safefs v0.2.0, where an empty `diskcache.Cache.Dir` is an error; `Client` still treats an empty `CacheDir` as caching off.

## [0.1.0] - 2026-10-29

### Added

- `Release`, `Asset`, `FetchReleases` and `Client` with a disk-cached, stale-on-failure `Releases`, `CachedReleases`,
  `ReleaseNotes` and `ReleasesBetween`.
- `Select` with a `keep` predicate, `Installable`, `Shape` and `Key` for choosing and recognising release assets.
- `ZipNames`, a remote zip's file list over ranged requests.
- `Observe`, `RateLimited`, `CurrentRate` and `Rate` for GitHub's quota.
- `Auth`, `DefaultAuth` and `GHCLI` for an opt-in `gh` login; `ValidRepo` and `ErrInvalidRepo` for names.

[Unreleased]: https://github.com/Rethunk-Tech/go-github/compare/v0.3.0...HEAD
[0.3.0]: https://github.com/Rethunk-Tech/go-github/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/Rethunk-Tech/go-github/compare/v0.1.1...v0.2.0
[0.1.1]: https://github.com/Rethunk-Tech/go-github/compare/v0.1.0...v0.1.1
[0.1.0]: https://github.com/Rethunk-Tech/go-github/releases/tag/v0.1.0
