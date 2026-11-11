# Security

## Supported versions

The latest tagged release. There are no long-lived release branches, so a fix ships in the next tag
rather than being backported.

## Reporting a vulnerability

Use GitHub's private vulnerability reporting on this repository: <https://github.com/Rethunk-Tech/go-github/security/advisories/new>, or the **Security** tab, then **Report a vulnerability**. The report stays private between you and us until a fix ships.

If private reporting is unavailable, email [oss@rethunk.tech](mailto:oss@rethunk.tech) with **[SECURITY]** in the subject.

Please do not open a public issue, pull request or discussion for a suspected vulnerability.

A useful report has the affected repository and version (or commit), steps to reproduce, what an attacker gets and under which conditions, and whether anyone else knows.

## What counts as a vulnerability

A vulnerability is a flaw that lets someone cross a boundary they should not: run code, read or change data they have no right to, escape a sandbox or path restriction, or leak a credential. A bug is anything else that misbehaves, such as a crash on your own input, wrong output or a slow path; open an ordinary issue for those. If you are unsure which one you have, report it privately and we will say.

## Coordinated disclosure

We handle the report inside the private advisory, so you see each status change and can comment on the fix. The advisory goes public when the fix ships, or 90 days after your report, whichever comes first. If a fix is close, either of us can ask for more time in the thread. You are credited in the advisory unless you would rather not be.

## Response timelines

- We acknowledge your report within 3 business days.
- We give you an assessment (accepted, needs more information or declined, with reasons) within 10 business days.
- We aim to ship a fix within 90 days of the report.

## What is in scope

- `Client.Releases`, `ReleaseNotes`, `ReleasesBetween`: `owner`, `repo` and `tag` may come from a manifest or a user.
  They must not move the request URL (`ValidRepo`, tag escaping), and a refused name makes no request.
- `FetchReleases`, `ReleaseNotes`: the API answer is untrusted JSON. A body is read capped (8 MiB, 1 MiB for notes)
  and must not panic the decoder (`FuzzFetchReleases`).
- `Auth.Apply`: the token goes only to https `github.com` or `api.github.com`; a redirect to plain http is refused, and
  the token is never in an error.
- `ZipNames`: a remote zip's directory is untrusted; only the central directory is read, and a redirect from https to plain http is refused.
- Cache files: a name that is not a local path under `CacheDir` is not read or written (go-safefs `diskcache`).

## What is out of scope

- Choosing which asset to download and verifying its contents (`Asset.Digest` is GitHub's claim, not checked here).
- The program named by `Auth.Run`: the caller sets it, and `GHCLI` runs `gh auth token` only when asked.
- A hostile `APIBase` or `HTTP` client: the caller supplies both.

## Safe harbour

We will not pursue legal action against good-faith research that avoids privacy violations, data destruction and disruption of services, stops at a proof of concept, and keeps the details private until the advisory is published. Rethunk.Tech, LLC maintains this repository.
