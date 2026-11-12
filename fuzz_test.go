package github

import (
	"archive/zip"
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func FuzzValidRepo(f *testing.F) {
	for _, s := range []string{
		"owner/repo",
		"a/b",
		".",
		"..",
		"a/.",
		"a/..",
		"./b",
		"../b",
		"",
		"/",
		"a/",
		"/b",
		"a/b/c",
		"A.b/C_d-1",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if !ValidRepo(s) {
			return
		}
		owner, name, found := strings.Cut(s, "/")
		if !found || name == "" || strings.Contains(name, "/") {
			t.Fatalf("accepted %q", s)
		}
		if strings.Trim(owner, ".") == "" || strings.Trim(name, ".") == "" {
			t.Fatalf("accepted dot name %q", s)
		}
	})
}

func FuzzShape(f *testing.F) {
	for _, s := range []string{
		"MyMod-1.2.0-linux.zip",
		"MyMod-v1.3-linux.zip",
		"",
		"a",
		"foo.tar.gz",
		"checksums.txt",
		"mod_1.0.0_windows_amd64.exe",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		got := Shape(name)
		if again := Shape(got); again != got {
			t.Fatalf("Shape(%q)=%q; Shape=%q", name, got, again)
		}
	})
}

func FuzzInstallable(f *testing.F) {
	for _, s := range []struct{ name, goos string }{
		{"MyMod-1.2.0-linux.zip", "linux"},
		{"MyMod-1.2.0-linux.zip", "windows"},
		{"checksums.txt", "linux"},
		{"src.tar.gz", "darwin"},
		{"foo.exe", "windows"},
	} {
		f.Add(s.name, s.goos)
	}
	f.Fuzz(func(t *testing.T, name, goos string) {
		in := []Asset{{Name: name}}
		out := Installable(in, goos)
		if len(out) > len(in) {
			t.Fatalf("Installable grew from %d to %d", len(in), len(out))
		}
	})
}

func zipSeed(names ...string) []byte {
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for _, n := range names {
		w, err := zw.Create(n)
		if err != nil {
			continue
		}
		_, _ = w.Write([]byte("x"))
	}
	_ = zw.Close()
	return buf.Bytes()
}

func FuzzZipNames(f *testing.F) {
	f.Add(zipSeed("a.txt"))
	f.Add(zipSeed("dir/b.txt", "c.txt"))
	f.Add([]byte("PK\x03\x04"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			data = data[:64<<10]
		}
		hc := &http.Client{Transport: zipRT{data}}
		_, _ = ZipNames(context.Background(), hc, Asset{Name: "x.zip", URL: "https://example.com/x.zip", Size: int64(len(data))})
	})
}

type zipRT struct{ data []byte }

func (z zipRT) RoundTrip(req *http.Request) (*http.Response, error) {
	start, end, ok := parseBytesRange(req.Header.Get("Range"), int64(len(z.data)))
	hdr := make(http.Header)
	hdr.Set("Accept-Ranges", "bytes")
	body := z.data
	code := http.StatusOK
	if ok {
		body = z.data[start : end+1]
		hdr.Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, len(z.data)))
		code = http.StatusPartialContent
	}
	hdr.Set("Content-Length", strconv.Itoa(len(body)))
	return &http.Response{
		StatusCode:    code,
		Header:        hdr,
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       req,
		Proto:         "HTTP/1.1",
		ProtoMajor:    1,
		ProtoMinor:    1,
	}, nil
}

func parseBytesRange(h string, size int64) (start, end int64, ok bool) {
	h, ok = strings.CutPrefix(h, "bytes=")
	if !ok || size <= 0 {
		return 0, 0, false
	}
	a, b, _ := strings.Cut(h, "-")
	if a == "" {
		return 0, 0, false
	}
	start, err := strconv.ParseInt(a, 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false
	}
	end = size - 1
	if b != "" {
		end, err = strconv.ParseInt(b, 10, 64)
		if err != nil || end < start {
			return 0, 0, false
		}
		if end >= size {
			end = size - 1
		}
	}
	return start, end, true
}
