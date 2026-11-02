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
	ctx := context.Background()

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

	if _, err := c.CreateRelease(context.Background(), "o", "r", NewRelease{Tag: "v1"}); !errors.Is(err, ErrNoToken) {
		t.Fatalf("no login: %v", err)
	}
	withToken(t, "tok")
	_, err := c.CreateRelease(context.Background(), "o", "r", NewRelease{Tag: "v1"})
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
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, url, nil)
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
	if _, err := (&Client{APIBase: srv.URL}).CreateRelease(context.Background(), "o", "r", NewRelease{Tag: "v1"}); err != nil || auth != "" {
		t.Fatalf("err %v, Authorization %q sent to a non-GitHub host", err, auth)
	}
}
