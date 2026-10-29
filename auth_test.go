package github

import (
	"context"
	"errors"
	"net/http"
	"testing"
)

const secret = "gho_testsecret123"

func req(t *testing.T, url string) *http.Request {
	t.Helper()
	r, err := http.NewRequestWithContext(context.Background(), http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

func TestAuthApply(t *testing.T) {
	t.Parallel()
	present := &Auth{Run: func(context.Context) ([]byte, error) { return []byte(secret + "\n"), nil }}
	absent := &Auth{Run: func(context.Context) ([]byte, error) { return nil, nil }}
	failing := &Auth{Run: func(context.Context) ([]byte, error) { return nil, errors.New("gh: not logged in " + secret) }}

	r := req(t, "https://api.github.com/repos/o/r/releases")
	present.Apply(r)
	if got := r.Header.Get("Authorization"); got != "Bearer "+secret {
		t.Fatalf("present: header %q", got)
	}
	for name, a := range map[string]*Auth{"zero": {}, "absent": absent, "failing": failing} {
		r := req(t, "https://github.com/o/r/releases/download/v1/a.zip")
		a.Apply(r)
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("%s: header %q", name, got)
		}
	}
	other := req(t, "https://example.com/a.zip")
	present.Apply(other)
	if other.Header.Get("Authorization") != "" {
		t.Fatal("token sent to a non-GitHub host")
	}
	plain := req(t, "http://api.github.com/repos/o/r/releases")
	present.Apply(plain)
	if plain.Header.Get("Authorization") != "" {
		t.Fatal("token sent over http")
	}
}

func TestAuthCachesTheAnswer(t *testing.T) {
	t.Parallel()
	n := 0
	a := &Auth{Run: func(context.Context) ([]byte, error) { n++; return nil, errors.New("x") }}
	for range 3 {
		a.Apply(req(t, "https://api.github.com/x"))
	}
	if n != 1 {
		t.Fatalf("Run ran %d times", n)
	}
	if a.LoggedIn(t.Context()) {
		t.Fatal("a failed Run counted as logged in")
	}
}
