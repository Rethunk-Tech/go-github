package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPathWalkingTag(t *testing.T) {
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits++
	}))
	t.Cleanup(srv.Close)
	c := &Client{HTTP: srv.Client(), APIBase: srv.URL}

	for _, tag := range []string{"../x", "foo/../../secret"} {
		hits = 0
		if _, err := c.ReleaseNotes(context.Background(), "owner", "repo", tag); !errors.Is(err, ErrNoRelease) {
			t.Fatalf("ReleaseNotes %q: err=%v", tag, err)
		}
		if hits != 0 {
			t.Fatalf("ReleaseNotes %q: sent %d requests", tag, hits)
		}
		hits = 0
		if _, _, err := c.ReleaseByTag(context.Background(), "owner", "repo", tag); !errors.Is(err, ErrNoRelease) {
			t.Fatalf("ReleaseByTag %q: err=%v", tag, err)
		}
		if hits != 0 {
			t.Fatalf("ReleaseByTag %q: sent %d requests", tag, hits)
		}
	}

	hits = 0
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits++
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"body":"notes"}`))
	}))
	t.Cleanup(srv2.Close)
	c2 := &Client{HTTP: srv2.Client(), APIBase: srv2.URL}
	notes, err := c2.ReleaseNotes(context.Background(), "owner", "repo", "release/1.0")
	if err != nil {
		t.Fatalf("release/1.0: %v", err)
	}
	if notes != "notes" {
		t.Fatalf("release/1.0 notes=%q", notes)
	}
	if hits == 0 {
		t.Fatal("release/1.0: no request")
	}
}
