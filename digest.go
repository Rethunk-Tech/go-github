package github

import (
	"crypto/sha256"
	"errors"
	"strings"

	"github.com/Rethunk-Tech/go-safefs"
)

// ErrDigestMismatch says a downloaded file is not the one GitHub published a digest for.
var ErrDigestMismatch = errors.New("the file does not match the digest GitHub published")

// VerifyDigest checks the file at path against a.Digest. An asset with no digest, or one in an algorithm other than
// sha256 (the only one GitHub publishes), passes: there is nothing to hold the file to. A different hash is
// ErrDigestMismatch.
func (a Asset) VerifyDigest(path string) error {
	want, ok := strings.CutPrefix(strings.ToLower(strings.TrimSpace(a.Digest)), "sha256:")
	if !ok {
		return nil
	}
	got, err := safefs.HashFile(path, sha256.New())
	if err != nil {
		return err
	}
	if got != want {
		return ErrDigestMismatch
	}
	return nil
}
