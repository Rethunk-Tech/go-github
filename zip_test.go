package github

import (
	"archive/zip"
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"slices"
	"sync/atomic"
	"testing"
	"time"
)

func TestZipNamesReadsOnlyTheDirectory(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range []string{"Mod/manifest.json", "Mod/Mod.dll"} {
		w, _ := zw.CreateHeader(&zip.FileHeader{Name: n, Method: zip.Store})
		_, _ = w.Write(bytes.Repeat([]byte("x"), 200<<10))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	served := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Range") == "" {
			t.Error("asked for the whole file")
		}
		http.ServeContent(counter{w, &served}, r, "a.zip", time.Time{}, bytes.NewReader(buf.Bytes()))
	}))
	t.Cleanup(srv.Close)
	got, err := ZipNames(t.Context(), srv.Client(), Asset{Name: "a.zip", URL: srv.URL, Size: int64(buf.Len())})
	if err != nil || !slices.Equal(got, []string{"Mod/manifest.json", "Mod/Mod.dll"}) {
		t.Fatalf("names %v %v", got, err)
	}
	if served > 64<<10 {
		t.Errorf("read %d bytes of a %d byte zip", served, buf.Len())
	}
}

func TestZipNamesRefusesWhatIsNotAKnownZip(t *testing.T) {
	t.Parallel()
	for _, a := range []Asset{{Name: "a.7z", Size: 10}, {Name: "a.zip"}} {
		if _, err := ZipNames(t.Context(), nil, a); err == nil {
			t.Errorf("%+v was read", a)
		}
	}
}

type counter struct {
	http.ResponseWriter
	n *int
}

func (c counter) Write(b []byte) (int, error) {
	*c.n += len(b)
	return c.ResponseWriter.Write(b)
}

// Not parallel: DefaultAuth is process-wide.
func TestZipNamesLendsTheToken(t *testing.T) {
	prev := DefaultAuth
	DefaultAuth = &Auth{Run: func(context.Context) ([]byte, error) { return []byte("secret"), nil }}
	t.Cleanup(func() { DefaultAuth = prev })

	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	if _, err := zw.Create("a.txt"); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			http.NotFound(w, r)
			return
		}
		http.ServeContent(w, r, "a.zip", time.Time{}, bytes.NewReader(buf.Bytes()))
	}))
	t.Cleanup(srv.Close)
	inner := srv.Client()
	hc := &http.Client{Transport: roundTripperFunc(func(req *http.Request) (*http.Response, error) {
		u, err := url.Parse(srv.URL)
		if err != nil {
			return nil, err
		}
		req = req.Clone(req.Context())
		req.URL.Scheme, req.URL.Host, req.Host = u.Scheme, u.Host, u.Host
		return inner.Transport.RoundTrip(req)
	})}

	got, err := ZipNames(t.Context(), hc, Asset{
		Name: "a.zip",
		URL:  "https://github.com/o/r/releases/download/v1/a.zip",
		Size: int64(buf.Len()),
	})
	if err != nil || !slices.Equal(got, []string{"a.txt"}) {
		t.Fatalf("names %v %v", got, err)
	}
}

type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestZipNamesRefusesAnHTTPSDowngradeRedirect(t *testing.T) {
	t.Parallel()
	var plainHits atomic.Int32
	plain := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { plainHits.Add(1) }))
	t.Cleanup(plain.Close)
	secure := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, plain.URL, http.StatusFound)
	}))
	t.Cleanup(secure.Close)
	_, err := ZipNames(t.Context(), secure.Client(), Asset{Name: "a.zip", URL: secure.URL, Size: 1 << 20})
	if err == nil || plainHits.Load() != 0 {
		t.Fatalf("err = %v, plain hits = %d", err, plainHits.Load())
	}
}
