# Security

## Supported versions

The latest tagged release. There are no long-lived release branches, so a fix ships in the next tag
rather than being backported.

## Reporting a vulnerability

Use GitHub's **private vulnerability reporting** on this repository (Security → Report a
vulnerability). It stays private until a fix ships.

Please do not open a public issue for a suspected vulnerability first.

## What is in scope

- `Client.Releases`, `ReleaseNotes`, `ReleasesBetween`: `owner`, `repo` and `tag` may come from a manifest or a user.
  They must not move the request URL (`ValidRepo`, tag escaping), and a refused name makes no request.
- `FetchReleases`, `ReleaseNotes`: the API answer is untrusted JSON. A body is read capped (8 MiB, 1 MiB for notes)
  and must not panic the decoder (`FuzzFetchReleases`).
- `Auth.Apply`: the token goes only to https `github.com` or `api.github.com`; a redirect to plain http is refused, and
  the token is never in an error.
- `ZipNames`: a remote zip's directory is untrusted; only the central directory is read.
- Cache files: a name that is not a local path under `CacheDir` is not read or written (go-safefs `diskcache`).

## What is out of scope

- Choosing which asset to download and verifying its contents (`Asset.Digest` is GitHub's claim, not checked here).
- The program named by `Auth.Run`: the caller sets it, and `GHCLI` runs `gh auth token` only when asked.
- A hostile `APIBase` or `HTTP` client: the caller supplies both.
