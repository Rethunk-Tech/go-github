package github

import (
	"archive/zip"
	"bytes"
	"net/http"
	"net/http/httptest"
	"slices"
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
