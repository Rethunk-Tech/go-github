// Package github reads a repository's GitHub releases for programs that install from release assets. Lookups are
// cached on disk, because the API allows 60 calls an hour per IP address without a token and 5,000 with one, and a
// stale answer stands in when GitHub cannot be reached or the limit is reached. The package also picks the release a
// version names, narrows its assets to the ones a user of this OS would install, recognises the same download
// across releases, lists a remote zip's files without downloading it, and can lend requests the user's gh login.
package github
