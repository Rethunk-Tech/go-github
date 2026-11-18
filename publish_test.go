package github

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/Rethunk-Tech/go-httpkit"
)

// withToken installs a fake login for the test; the tests using it are not parallel, since DefaultAuth is global.
func withToken(t *testing.T, tok string) {
	t.Helper()
	old := DefaultAuth
	DefaultAuth = &Auth{Run: func(context.Context) ([]byte, error) { return []byte(tok + "\n"), nil }}
	t.Cleanup(func() { DefaultAuth = old })
}

func TestPublishFlow(t *testing.T) {
	withToken(t, "tok")
	var srv *httptest.Server
	var created map[string]any
	var uploadName, uploadType string
	var uploaded []byte
	var uploadLen int64
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/repos/o/r/releases":
			_ = json.NewDecoder(r.Body).Decode(&created)
			_, _ = w.Write([]byte(`{"id":7,"tag_name":"v1","html_url":"h","upload_url":"` + srv.URL + `/up/7/assets{?name,label}"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/up/7/assets":
			uploadName, uploadType, uploadLen = r.URL.Query().Get("name"), r.Header.Get("Content-Type"), r.ContentLength
			uploaded, _ = io.ReadAll(r.Body)
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"id":9,"name":"` + uploadName + `","size":` + strconv.Itoa(len(uploaded)) + `}`))
		case r.URL.Path == "/repos/o/r/releases/tags/v1":
			_, _ = w.Write([]byte(`{"id":7,"tag_name":"v1"}`))
		default:
			http.Error(w, `{"message":"Not Found"}`, http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	c := &Client{APIBase: srv.URL}
	ctx := t.Context()

	rel, err := c.CreateRelease(ctx, "o", "r", NewRelease{Tag: "v1", Name: "One", Body: "notes", Draft: true})
	if err != nil || rel.ID != 7 || rel.HTMLURL != "h" {
		t.Fatalf("CreateRelease = %+v, %v", rel, err)
	}
	if created["tag_name"] != "v1" || created["name"] != "One" || created["body"] != "notes" || created["draft"] != true || created["prerelease"] != false {
		t.Fatalf("payload = %v", created)
	}
	if _, ok := created["target_commitish"]; ok {
		t.Fatal("target_commitish sent while unset")
	}
	if _, err := c.CreateRelease(ctx, "o", "r", NewRelease{Tag: "v1", Target: "abc"}); err != nil {
		t.Fatal(err)
	}
	if created["target_commitish"] != "abc" {
		t.Fatalf("target_commitish = %v", created["target_commitish"])
	}

	path := filepath.Join(t.TempDir(), "m.zip")
	if err := os.WriteFile(path, []byte("zipdata"), 0o600); err != nil {
		t.Fatal(err)
	}
	a, err := c.UploadAsset(ctx, rel, path, "application/zip")
	if err != nil || a.ID != 9 || uploadName != "m.zip" || string(uploaded) != "zipdata" || uploadLen != 7 || uploadType != "application/zip" {
		t.Fatalf("UploadAsset = %+v, %v; name %q body %q len %d type %q", a, err, uploadName, uploaded, uploadLen, uploadType)
	}

	if got, found, err := c.ReleaseByTag(ctx, "o", "r", "v1"); err != nil || !found || got.ID != 7 {
		t.Fatalf("ReleaseByTag v1 = %+v, %v, %v", got, found, err)
	}
	if _, found, err := c.ReleaseByTag(ctx, "o", "r", "v2"); err != nil || found {
		t.Fatalf("ReleaseByTag v2 = %v, %v", found, err)
	}
}

func TestPublishErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"Validation Failed"}`, http.StatusUnprocessableEntity)
	}))
	t.Cleanup(srv.Close)
	c := &Client{APIBase: srv.URL}

	if _, err := c.CreateRelease(t.Context(), "o", "r", NewRelease{Tag: "v1"}); !errors.Is(err, ErrNoToken) {
		t.Fatalf("no login: %v", err)
	}
	withToken(t, "tok")
	_, err := c.CreateRelease(t.Context(), "o", "r", NewRelease{Tag: "v1"})
	var se *httpkit.StatusError
	if !errors.As(err, &se) || se.Code != 422 || err.Error() != "server answered 422 Unprocessable Entity: Validation Failed" {
		t.Fatalf("422: %v", err)
	}
}

func TestAuthOnlyToGitHub(t *testing.T) {
	t.Parallel()
	a := &Auth{Run: func(context.Context) ([]byte, error) { return []byte("tok"), nil }}
	for url, want := range map[string]string{
		"https://api.github.com/repos/o/r/releases":       "Bearer tok",
		"https://uploads.github.com/repos/o/r/releases/1": "Bearer tok",
		"http://api.github.com/x":                         "",
		"https://evil.example/repos/o/r/releases":         "",
		"https://api.github.com.evil.example/x":           "",
	} {
		req, _ := http.NewRequestWithContext(t.Context(), http.MethodPost, url, nil)
		a.Apply(req)
		if got := req.Header.Get("Authorization"); got != want {
			t.Errorf("%s: Authorization = %q, want %q", url, got, want)
		}
	}
}

func TestNoAuthToTestServer(t *testing.T) {
	withToken(t, "tok")
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	if _, err := (&Client{APIBase: srv.URL}).CreateRelease(t.Context(), "o", "r", NewRelease{Tag: "v1"}); err != nil || auth != "" {
		t.Fatalf("err %v, Authorization %q sent to a non-GitHub host", err, auth)
	}
}

// A caller who already gave up is told so, rather than being told it has no token.
func TestPublishCancelledContext(t *testing.T) {
	withToken(t, "unused-token")
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	c := &Client{}
	if _, err := c.CreateRelease(ctx, "o", "r", NewRelease{Tag: "v1"}); !errors.Is(err, context.Canceled) || errors.Is(err, ErrNoToken) {
		t.Fatalf("CreateRelease: %v", err)
	}
	if _, _, err := c.ReleaseByTag(ctx, "o", "r", "v1"); !errors.Is(err, context.Canceled) || errors.Is(err, ErrNoToken) {
		t.Fatalf("ReleaseByTag: %v", err)
	}
}

// A GitHub Enterprise install answers on the host its APIBase names, so publishing and reading there are
// authenticated too; the token still goes nowhere else.
func TestAuthToAPIBaseHost(t *testing.T) {
	const tok = "enterprise-token"
	withToken(t, tok)
	var seen []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		if r.Method == http.MethodGet && r.URL.Path == "/repos/o/r/releases" {
			_, _ = w.Write([]byte(`[]`))
			return
		}
		_, _ = w.Write([]byte(`{"id":1,"body":"notes"}`))
	}))
	t.Cleanup(srv.Close)
	c := &Client{HTTP: srv.Client(), APIBase: srv.URL}

	if _, err := c.CreateRelease(t.Context(), "o", "r", NewRelease{Tag: "v1"}); err != nil {
		t.Fatalf("CreateRelease: %v", err)
	}
	if _, _, err := c.ReleaseByTag(t.Context(), "o", "r", "v1"); err != nil {
		t.Fatalf("ReleaseByTag: %v", err)
	}
	if _, err := c.Releases(t.Context(), "o", "r"); err != nil {
		t.Fatalf("Releases: %v", err)
	}
	if len(seen) != 3 {
		t.Fatalf("saw %d requests, want 3", len(seen))
	}
	for _, a := range seen {
		if a != "Bearer "+tok {
			t.Fatalf("Authorization %q sent to the configured API host", a)
		}
	}
}

func TestPublishInvalidInputsMakeNoRequest(t *testing.T) {
	t.Parallel()
	var hits int
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits++
	}))
	t.Cleanup(srv.Close)
	c := &Client{APIBase: srv.URL}
	ctx := t.Context()

	if _, err := c.CreateRelease(ctx, "o/..", "r", NewRelease{Tag: "v1"}); !errors.Is(err, ErrInvalidRepo) {
		t.Fatalf("CreateRelease bad repo: %v", err)
	}
	if _, _, err := c.ReleaseByTag(ctx, "o/..", "r", "v1"); !errors.Is(err, ErrInvalidRepo) {
		t.Fatalf("ReleaseByTag bad repo: %v", err)
	}
	if _, err := c.UploadAsset(ctx, Release{Tag: "v1"}, "x", ""); err == nil {
		t.Fatal("empty upload URL succeeded")
	}
	missing := filepath.Join(t.TempDir(), "gone.bin")
	if _, err := c.UploadAsset(ctx, Release{UploadURL: srv.URL + "/up"}, missing, ""); err == nil {
		t.Fatal("missing file succeeded")
	}
	if hits != 0 {
		t.Fatalf("%d requests", hits)
	}
}

func TestPublishSurfacesTransportAndAnswerFaults(t *testing.T) {
	withToken(t, "tok")
	ctx := t.Context()

	dead := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	deadURL := dead.URL
	dead.Close()
	if _, err := (&Client{APIBase: deadURL}).CreateRelease(ctx, "o", "r", NewRelease{Tag: "v1"}); err == nil {
		t.Fatal("closed server succeeded")
	}

	limit := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("X-Ratelimit-Remaining", "0")
		w.Header().Set("X-Ratelimit-Reset", "1")
		w.WriteHeader(http.StatusForbidden)
	}))
	t.Cleanup(limit.Close)
	if _, err := (&Client{APIBase: limit.URL}).CreateRelease(ctx, "o", "r", NewRelease{Tag: "v1"}); !errors.Is(err, httpkit.ErrRateLimited) {
		t.Fatalf("rate limit: %v", err)
	}

	plain := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(plain.Close)
	err := mustCreate(t, plain.URL)
	var se *httpkit.StatusError
	if !errors.As(err, &se) || se.Code != http.StatusBadRequest || err.Error() != "server answered 400 Bad Request" {
		t.Fatalf("empty message: %v", err)
	}

	broken := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{`))
	}))
	t.Cleanup(broken.Close)
	var syn *json.SyntaxError
	if err := mustCreate(t, broken.URL); !errors.As(err, &syn) {
		t.Fatalf("truncated JSON: %v", err)
	}

	cut := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, rw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = rw.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 100\r\n\r\n{")
		_ = rw.Flush()
	}))
	t.Cleanup(cut.Close)
	if err := mustCreate(t, cut.URL); err == nil {
		t.Fatal("short body succeeded")
	}

	if _, err := (&Client{APIBase: "http://["}).CreateRelease(ctx, "o", "r", NewRelease{Tag: "v1"}); err == nil {
		t.Fatal("invalid API URL succeeded")
	}
	if _, _, err := (&Client{APIBase: "http://["}).ReleaseByTag(ctx, "o", "r", "v1"); err == nil {
		t.Fatal("ReleaseByTag invalid API URL succeeded")
	}

	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, `{"message":"Validation Failed"}`, http.StatusUnprocessableEntity)
	}))
	t.Cleanup(up.Close)
	path := filepath.Join(t.TempDir(), "a.bin")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err = (&Client{APIBase: up.URL}).UploadAsset(ctx, Release{UploadURL: up.URL + "/assets"}, path, "")
	if !errors.As(err, &se) || se.Code != http.StatusUnprocessableEntity {
		t.Fatalf("rejected upload: %v", err)
	}
}

func mustCreate(t *testing.T, base string) error {
	t.Helper()
	_, err := (&Client{APIBase: base}).CreateRelease(t.Context(), "o", "r", NewRelease{Tag: "v1"})
	return err
}
