package github

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Rethunk-Tech/go-httpkit"
	"github.com/Rethunk-Tech/go-safefs"
	"github.com/Rethunk-Tech/go-safefs/diskcache"
)

func server(t *testing.T, hits *atomic.Int32, h http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits.Add(1)
		h(w, r)
	}))
	t.Cleanup(srv.Close)
	return &Client{APIBase: srv.URL, CacheDir: t.TempDir()}
}

func serve(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte(releasesJSON)) }

func TestReleasesCacheAndStale(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	fail := false
	c := server(t, &hits, func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "boom", http.StatusInternalServerError)
			return
		}
		if r.URL.Path != "/repos/o/r/releases" {
			t.Errorf("path = %s", r.URL.Path)
		}
		serve(w, r)
	})
	now := time.Now().UTC()
	c.Now = func() time.Time { return now }
	if _, err := c.Releases(context.Background(), "o", "r"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Releases(context.Background(), "o", "r"); err != nil || hits.Load() != 1 {
		t.Fatalf("cache hit: hits = %d, err = %v", hits.Load(), err)
	}
	now = now.Add(2 * time.Hour)
	fail = true
	got, err := c.Releases(context.Background(), "o", "r")
	if err != nil || len(got) != 5 || hits.Load() != 2 {
		t.Fatalf("stale fallback = %d releases, hits %d, err %v", len(got), hits.Load(), err)
	}
	_, err = (&Client{APIBase: c.APIBase, CacheDir: t.TempDir()}).Releases(context.Background(), "o", "r")
	var se *httpkit.StatusError
	if !errors.As(err, &se) || se.Code != http.StatusInternalServerError {
		t.Fatalf("no cache and a failing API must be a *httpkit.StatusError: %v", err)
	}
}

func TestEmptyCacheDirFetchesEveryTime(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	c := server(t, &hits, serve)
	c.CacheDir = ""
	for range 2 {
		if _, err := c.Releases(context.Background(), "o", "r"); err != nil {
			t.Fatal(err)
		}
	}
	if hits.Load() != 2 {
		t.Fatalf("hits = %d", hits.Load())
	}
	if _, _, ok := c.CachedReleases("o", "r"); ok {
		t.Fatal("a disabled cache answered")
	}
}

func TestCachedReleasesNeverAsksAndOldFormatsAreFetchedAgain(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	c := server(t, &hits, serve)
	if _, _, ok := c.CachedReleases("o", "r"); ok {
		t.Fatal("an empty cache answered")
	}
	if _, err := c.Releases(context.Background(), "o", "r"); err != nil {
		t.Fatal(err)
	}
	got, whole, ok := c.CachedReleases("o", "r")
	if !ok || !whole || len(got) != 5 || hits.Load() != 1 {
		t.Fatalf("cached = %d releases, whole %v, ok %v, hits %d", len(got), whole, ok, hits.Load())
	}
	// A file stamped with an older format lacks fields the current one has, so the list is read again however young.
	path := filepath.Join(c.CacheDir, releasesName("o", "r"))
	if err := os.WriteFile(path, []byte(`{"fetched":"`+time.Now().UTC().Format(time.RFC3339)+`","value":[{"tag_name":"v0"}],"build":"1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := c.CachedReleases("o", "r"); ok {
		t.Fatal("an old format was read")
	}
	if got, err := c.Releases(context.Background(), "o", "r"); err != nil || len(got) != 5 || hits.Load() != 2 {
		t.Fatalf("old format: %d releases, hits %d, err %v", len(got), hits.Load(), err)
	}
	c.Now = func() time.Time { return time.Now().UTC().Add(-24 * time.Hour) }
	if _, err := c.Releases(context.Background(), "o", "r"); err != nil || hits.Load() != 3 {
		t.Fatalf("an answer stamped after a clock set back was served: hits %d, err %v", hits.Load(), err)
	}
}

func TestEarlierCacheShapesReadAsAbsent(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	c := server(t, &hits, serve)
	now := time.Now().UTC().Format(time.RFC3339)
	key := Key("o", "r", "", "")
	for name, body := range map[string]string{
		key + "-releases.json":                     `{"fetched":"` + now + `","releases":[{"tag_name":"v0"}]}`,
		releasesName("o", "r"):                     `{"fetched":"` + now + `","releases":[{"tag_name":"v0"}]}`,
		Key("o", "r", "v1", "") + "-notes.json":    `{"notes":"old"}`,
		Key("o", "r", "v2", "") + "-notes-v2.json": `{"notes":"old"}`,
	} {
		if err := os.WriteFile(filepath.Join(c.CacheDir, name), []byte(body), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, ok := c.CachedReleases("o", "r"); ok {
		t.Fatal("a release list without a value answered")
	}
	if got, err := c.Releases(context.Background(), "o", "r"); err != nil || len(got) != 5 || hits.Load() != 1 {
		t.Fatalf("releases: %d, hits %d, err %v", len(got), hits.Load(), err)
	}
	if notes, ok := diskcache.Peek[string](c.cache(), Key("o", "r", "v2", "")+"-notes-v2.json"); ok {
		t.Fatalf("notes without a value answered %q", notes.Value)
	}
	if notes, ok := diskcache.Peek[string](c.cache(), Key("o", "r", "v1", "")+"-notes-v2.json"); ok {
		t.Fatalf("an unversioned notes file answered %q", notes.Value)
	}
}

func TestRateLimit(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	c := server(t, &hits, func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Ratelimit-Remaining", "0")
		w.Header().Set("X-Ratelimit-Reset", "1790000000")
		http.Error(w, `{"message":"API rate limit exceeded"}`, http.StatusForbidden)
	})
	_, err := c.Releases(context.Background(), "o", "r")
	var rl *httpkit.RateLimitError
	if !errors.As(err, &rl) || !rl.Reset.Equal(time.Unix(1790000000, 0)) {
		t.Fatalf("err = %v", err)
	}
	// A file stamped with an older format still stands in when the lookup fails.
	if err := os.WriteFile(filepath.Join(c.CacheDir, releasesName("o", "r")), []byte(`{"fetched":"2025-01-01T00:00:00Z","value":[{"tag_name":"v0"}],"build":"1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := c.Releases(context.Background(), "o", "r"); err != nil || len(got) != 1 {
		t.Fatalf("old format under a rate limit: %d releases, err %v", len(got), err)
	}
}

func TestBodyOverTheCapIsTooLarge(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	c := server(t, &hits, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`["` + strings.Repeat("x", maxList) + `"]`))
	})
	if _, err := c.Releases(context.Background(), "o", "r"); !errors.Is(err, safefs.ErrTooLarge) {
		t.Fatalf("err = %v", err)
	}
}

func TestReleasesBetween(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	c := server(t, &hits, func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`[
 {"tag_name":"v2.2.0","body":"too new","published_at":"2026-03-01T00:00:00Z"},
 {"tag_name":"v2.1.0","body":"- fix crash\n- see https://x.test","published_at":"2026-02-01T10:00:00Z"},
 {"tag_name":"v2.0.5","draft":true,"body":"draft"},
 {"tag_name":"nightly","body":"not a version"},
 {"tag_name":"v2.0.0","body":"rewrite","published_at":"2026-01-01T00:00:00Z"},
 {"tag_name":"v1.0.0","body":"installed"}]`))
	})
	got, err := c.ReleasesBetween(context.Background(), "o", "r", "1.0.0", "2.1.0")
	if err != nil || len(got) != 2 || got[0].Tag != "v2.1.0" || got[1].Tag != "v2.0.0" || got[0].Published != "2026-02-01T10:00:00Z" {
		t.Fatalf("got %+v, %v", got, err)
	}
	if got, err = c.ReleasesBetween(context.Background(), "o", "r", "1.0.0", ""); err != nil || len(got) != 3 {
		t.Fatalf("no latest bound: %+v, %v", got, err)
	}
}

func TestReleaseNotes(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	var gotPath string
	fail := false
	c := server(t, &hits, func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.EscapedPath()
		if fail {
			http.Error(w, "boom", http.StatusBadGateway)
			return
		}
		_, _ = w.Write([]byte(`{"body":"  hi \n"}`))
	})
	fail = true
	if _, err := c.ReleaseNotes(context.Background(), "o", "r", "v1"); err == nil {
		t.Fatal("a failure was swallowed")
	}
	fail = false
	for range 2 {
		if notes, err := c.ReleaseNotes(context.Background(), "o", "r", "rel/1 x"); err != nil || notes != "hi" {
			t.Fatalf("notes %q, %v", notes, err)
		}
	}
	if hits.Load() != 2 || gotPath != "/repos/o/r/releases/tags/rel%2F1%20x" {
		t.Fatalf("hits %d (a failure is not cached, a hit is final), path %q", hits.Load(), gotPath)
	}
}

func TestInvalidInputsMakeNoRequest(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	c := server(t, &hits, serve)
	ctx := context.Background()
	for _, r := range [][2]string{{"me", ".."}, {"o", "r?x"}, {"o/r", "x"}, {"", ""}} {
		if _, err := c.Releases(ctx, r[0], r[1]); !errors.Is(err, ErrInvalidRepo) {
			t.Errorf("Releases(%q, %q): %v", r[0], r[1], err)
		}
		if _, err := c.ReleaseNotes(ctx, r[0], r[1], "v1"); !errors.Is(err, ErrInvalidRepo) {
			t.Errorf("ReleaseNotes(%q, %q): %v", r[0], r[1], err)
		}
		if _, err := c.ReleasesBetween(ctx, r[0], r[1], "1", "2"); !errors.Is(err, ErrInvalidRepo) {
			t.Errorf("ReleasesBetween(%q, %q): %v", r[0], r[1], err)
		}
	}
	for _, tag := range []string{"", ".", ".."} {
		if _, err := c.ReleaseNotes(ctx, "o", "r", tag); !errors.Is(err, ErrNoRelease) {
			t.Errorf("tag %q: %v", tag, err)
		}
	}
	if hits.Load() != 0 {
		t.Fatalf("%d requests", hits.Load())
	}
}

// Not parallel: the shown rate is process-wide.
func TestReleaseNotesRecordsTheRate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Ratelimit-Limit", "5000")
		w.Header().Set("X-Ratelimit-Remaining", "4321")
		w.Header().Set("X-Ratelimit-Reset", "1700000000")
		_, _ = w.Write([]byte(`{"body":"hi"}`))
	}))
	t.Cleanup(srv.Close)
	c := &Client{APIBase: srv.URL, CacheDir: t.TempDir()}
	if _, err := c.ReleaseNotes(context.Background(), "o", "r", "v1"); err != nil {
		t.Fatal(err)
	}
	if r := CurrentRate(); !r.Known || r.Remaining != 4321 {
		t.Fatalf("rate %+v", r)
	}
}

func TestReleaseCallsRefuseARedirectToPlainHTTP(t *testing.T) {
	t.Parallel()
	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { _, _ = w.Write([]byte("[]")) }))
	t.Cleanup(plain.Close)
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL, http.StatusFound)
	}))
	t.Cleanup(secure.Close)
	if _, err := FetchReleases(context.Background(), secure.Client(), secure.URL); err == nil || !strings.Contains(err.Error(), "refusing a redirect") {
		t.Fatalf("FetchReleases followed it: %v", err)
	}
	c := &Client{HTTP: secure.Client(), APIBase: secure.URL, CacheDir: t.TempDir()}
	if _, err := c.ReleaseNotes(context.Background(), "o", "r", "v1"); err == nil || !strings.Contains(err.Error(), "refusing a redirect") {
		t.Fatalf("ReleaseNotes followed it: %v", err)
	}
}

// A caller that reports reachability wraps its client's transport in httpkit.Transport; every API call must reach it.
func TestRequestsReachTheCallersTransport(t *testing.T) {
	t.Parallel()
	var hits atomic.Int32
	c := server(t, &hits, serve)
	var seen atomic.Int32
	c.HTTP = &http.Client{Transport: &httpkit.Transport{OnResult: func(_ *http.Request, resp *http.Response, err error) {
		if err == nil && resp.StatusCode == http.StatusOK {
			seen.Add(1)
		}
	}}}
	if _, err := c.Releases(context.Background(), "o", "r"); err != nil {
		t.Fatal(err)
	}
	if _, err := c.ReleaseNotes(context.Background(), "o", "r", "v1"); err == nil {
		t.Fatal("the fixture serves a list, not a release: decoding must fail")
	}
	if seen.Load() != 2 {
		t.Fatalf("OnResult saw %d answers", seen.Load())
	}
}
