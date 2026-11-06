package github

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/Rethunk-Tech/go-httpkit"
)

func TestRepoInfo(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	c := server(t, &hits, func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/repos/o/r/readme":
			if r.Header.Get("Accept") != "application/vnd.github.raw" {
				t.Errorf("accept = %q", r.Header.Get("Accept"))
			}
			_, _ = w.Write([]byte("# hi"))
		case "/repos/o/r":
			_, _ = w.Write([]byte(`{"archived":true}`))
		case "/licenses/mpl-2.0":
			_, _ = w.Write([]byte(`{"body":"Mozilla"}`))
		default:
			http.NotFound(w, r)
		}
	})
	ctx := t.Context()
	if s, err := c.Readme(ctx, "o", "r"); err != nil || s != "# hi" {
		t.Fatalf("Readme = %q, %v", s, err)
	}
	if _, err := c.Readme(ctx, "o", "none"); !errors.Is(err, httpkit.ErrNotFound) {
		t.Fatalf("missing readme err = %v", err)
	}
	if a, err := c.Archived(ctx, "o", "r"); err != nil || !a {
		t.Fatalf("Archived = %v, %v", a, err)
	}
	if s, err := c.LicenseText(ctx, "mpl-2.0"); err != nil || s != "Mozilla" {
		t.Fatalf("LicenseText = %q, %v", s, err)
	}
	if _, err := c.LicenseText(ctx, "nope"); !errors.Is(err, httpkit.ErrNotFound) {
		t.Fatalf("unknown license err = %v", err)
	}
	before := hits.Load()
	if _, err := c.LicenseText(ctx, "../x"); !errors.Is(err, ErrInvalidLicense) {
		t.Fatalf("bad key err = %v", err)
	}
	if _, err := c.Readme(ctx, "o/..", "r"); !errors.Is(err, ErrInvalidRepo) || hits.Load() != before {
		t.Fatalf("bad repo err = %v", err)
	}
}

func TestVerifyDigest(t *testing.T) {
	t.Parallel()
	p := filepath.Join(t.TempDir(), "f")
	if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256([]byte("data"))
	good := "sha256:" + hex.EncodeToString(sum[:])
	for digest, want := range map[string]error{good: nil, "": nil, "md5:00": nil, "sha256:00": ErrDigestMismatch} {
		if err := (Asset{Digest: digest}).VerifyDigest(p); !errors.Is(err, want) {
			t.Errorf("%q: err = %v, want %v", digest, err, want)
		}
	}
}
