package github

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/Rethunk-Tech/go-httpkit"
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
		"me/..",
		"./mod",
		"me/.",
		"o/r?x",
		"o/r#x",
		"me/mod.cfg",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, s string) {
		if len(s) > 4<<10 {
			s = s[:4<<10]
		}
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
		"MyMod v1.3 linux.ZIP",
		"SMAPI 4.1.10 installer.zip",
		"Mod_1_2_3-win64.7z",
		"MelonLoader.Installer.exe",
		"app-v2-x86_64.AppImage",
		"",
		"a",
		"1.2.3",
		"foo.tar.gz",
		"checksums.txt",
		"checksums.sha256",
		"mod_1.0.0_windows_amd64.exe",
		"name.with.dots-1.0.tar.gz",
		"mod-v10-v11.zip",
		"mod-2024-01-02-nightly.zip",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, name string) {
		if len(name) > 4<<10 {
			name = name[:4<<10]
		}
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
		{"Mod-1.2-source.zip", "linux"},
		{"checksums.txt", "linux"},
		{"checksums.zip", "linux"},
		{"src.tar.gz", "darwin"},
		{"src.zip", "linux"},
		{"foo.exe", "windows"},
		{"Mod-1.2-win64.zip", "linux"},
		{"Mod-1.2-macOS.zip", "windows"},
		{"Mod-Windows.zip", "linux"},
	} {
		f.Add(s.name, s.goos)
	}
	f.Fuzz(func(t *testing.T, name, goos string) {
		if len(name) > 4<<10 {
			name = name[:4<<10]
		}
		if len(goos) > 64 {
			goos = goos[:64]
		}
		in := []Asset{{Name: name}}
		out := Installable(in, goos)
		if len(out) > len(in) {
			t.Fatalf("Installable grew from %d to %d", len(in), len(out))
		}
		for _, a := range out {
			if a.Name != name {
				t.Fatalf("invented %q from %q", a.Name, name)
			}
		}
	})
}

func FuzzFetchReleases(f *testing.F) {
	f.Add([]byte(`[{"tag_name":"v1.0.0","prerelease":false,"assets":[{"name":"a.zip","browser_download_url":"https://github.com/o/r/releases/download/v1.0.0/a.zip","size":3,"digest":"sha256:ab"}]}]`))
	f.Add([]byte(`{"message":"Not Found"}`))
	f.Add([]byte(`[{"assets":null}]`))
	f.Add([]byte(`[`))
	f.Add([]byte(releasesJSON))
	f.Fuzz(func(t *testing.T, body []byte) {
		if len(body) > 64<<10 {
			body = body[:64<<10]
		}
		hc := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return &http.Response{StatusCode: http.StatusOK, Body: io.NopCloser(bytes.NewReader(body)), Header: http.Header{}}, nil
		})}
		all, err := FetchReleases(t.Context(), hc, "http://github.test/repos/o/r/releases")
		if err != nil {
			return
		}
		r, assets, selErr := Select(all, "", false, nil)
		if selErr == nil {
			if r.Draft {
				t.Fatal("Select picked a draft")
			}
			if len(assets) == 0 {
				t.Fatal("Select succeeded with no assets")
			}
		} else if !errors.Is(selErr, ErrNoRelease) && !errors.Is(selErr, ErrNoAsset) {
			t.Fatalf("Select: %v", selErr)
		}
	})
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

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
	f.Add(zipSeed("Mod/manifest.json", "Mod/Mod.dll"))
	f.Add([]byte("PK\x03\x04"))
	f.Add([]byte{})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > 64<<10 {
			data = data[:64<<10]
		}
		hc := &http.Client{Transport: zipRT{data}}
		names, err := ZipNames(t.Context(), hc, Asset{Name: "x.zip", URL: "https://example.com/x.zip", Size: int64(len(data))})
		if err != nil {
			return
		}
		zr, zerr := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if zerr != nil {
			t.Fatalf("ZipNames=%v but zip.NewReader: %v", names, zerr)
		}
		if len(names) != len(zr.File) {
			t.Fatalf("ZipNames %d names, zip has %d", len(names), len(zr.File))
		}
		for i, n := range names {
			if n != zr.File[i].Name {
				t.Fatalf("names[%d]=%q, zip %q", i, n, zr.File[i].Name)
			}
		}
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

func FuzzTagMovesPath(f *testing.F) {
	for _, s := range []string{
		"v1.0.0",
		"../x",
		"foo/../../secret",
		"release/1.0",
		".",
		"..",
		"",
		"a/b",
		"foo/./bar",
		"foo//bar",
	} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, tag string) {
		if len(tag) > 4<<10 {
			tag = tag[:4<<10]
		}
		if tagMovesPath(tag) {
			return
		}
		escaped := url.PathEscape(tag)
		joined := "/releases/tags/" + escaped
		if path.Clean(joined) != path.Clean("/releases/tags") && !strings.HasPrefix(path.Clean(joined)+"/", "/releases/tags/") {
			t.Fatalf("accepted walking tag %q -> %q", tag, path.Clean(joined))
		}
		for e := range strings.SplitSeq(escaped, "/") {
			if e == "" || e == "." || e == ".." {
				t.Fatalf("accepted %q which PathEscape left as %q", tag, escaped)
			}
		}
	})
}

func FuzzNextLink(f *testing.F) {
	f.Add(`<https://api.github.com/repos/o/r/releases?per_page=30&page=2>; rel="next", <x>; rel="last"`)
	f.Add(`<https://evil.example/x>; rel="next"`)
	f.Add(`<SRV>; rel="next", <>; rel="next"`)
	f.Add(``)
	f.Add(`not a link`)
	f.Add(`<http://api.github.com/x>; rel="next"`)
	f.Fuzz(func(t *testing.T, header string) {
		if len(header) > 8<<10 {
			header = header[:8<<10]
		}
		got := nextLink(header)
		if got == "" {
			return
		}
		if !strings.Contains(header, `rel="next"`) {
			t.Fatalf("nextLink(%q)=%q without rel=next", header, got)
		}
		base := "https://api.github.com/repos/o/r"
		if !sameOrigin(base, got) {
			return
		}
		u, err := url.Parse(got)
		if err != nil || u.Scheme != "https" || u.Host != "api.github.com" {
			t.Fatalf("sameOrigin accepted %q", got)
		}
	})
}

func FuzzLicenseKey(f *testing.F) {
	for _, s := range []string{"mpl-2.0", "mit", "../x", ".", "..", "", "MIT", "gpl-3.0", "a", "nope"} {
		f.Add(s)
	}
	f.Fuzz(func(t *testing.T, key string) {
		if len(key) > 256 {
			key = key[:256]
		}
		if !licenseKey.MatchString(key) || key == "." || key == ".." {
			return
		}
		u, err := url.Parse("https://api.github.com/licenses/" + url.PathEscape(key))
		if err != nil {
			t.Fatalf("accepted %q: %v", key, err)
		}
		if u.Host != "api.github.com" {
			t.Fatalf("host moved: %q -> %q", key, u.Host)
		}
		cleaned := path.Clean(u.Path)
		if cleaned != "/licenses" && !strings.HasPrefix(cleaned+"/", "/licenses/") {
			t.Fatalf("path walked: %q -> %q", key, cleaned)
		}
		for e := range strings.SplitSeq(strings.TrimPrefix(u.EscapedPath(), "/licenses/"), "/") {
			if e == ".." || e == "." || e == "" {
				t.Fatalf("path element %q in %q", e, u.EscapedPath())
			}
		}
	})
}

var keySafe = regexp.MustCompile(`^[a-z0-9][a-z0-9.-]*$`)

func FuzzKey(f *testing.F) {
	f.Add("o", "r", "v1", "m.zip")
	f.Add("Me", "Mod", "v1.0", "mod.zip")
	f.Add("a-b", "c", "", "")
	f.Add("Pathoschild", "SMAPI", "4.1.10", "SMAPI 4.1.10 installer.zip")
	f.Add("_a", "b_", "", "?")
	f.Add("", "", "", "")
	f.Fuzz(func(t *testing.T, owner, repo, tag, asset string) {
		if len(owner) > 1<<10 {
			owner = owner[:1<<10]
		}
		if len(repo) > 1<<10 {
			repo = repo[:1<<10]
		}
		if len(tag) > 1<<10 {
			tag = tag[:1<<10]
		}
		if len(asset) > 1<<10 {
			asset = asset[:1<<10]
		}
		k := Key(owner, repo, tag, asset)
		if !keySafe.MatchString(k) {
			t.Fatalf("Key(%q,%q,%q,%q)=%q is not a file name", owner, repo, tag, asset, k)
		}
		if len(k) > 128 {
			t.Fatalf("Key is %d bytes: %q", len(k), k)
		}
		if Key(owner, repo, tag, asset) != k {
			t.Fatal("Key is not stable")
		}
	})
}

func FuzzSelect(f *testing.F) {
	f.Add("v2.1.0", "v1.0.0", false)
	f.Add("", "v2.0.0", true)
	f.Add("9.9.9", "", false)
	f.Add("V3.0.0-beta", "v3.0.0-beta", false)
	f.Add("2.0.0", "v2.0.0", false)
	f.Fuzz(func(t *testing.T, version, tag string, prerelease bool) {
		if len(version) > 256 {
			version = version[:256]
		}
		if len(tag) > 256 {
			tag = tag[:256]
		}
		all := []Release{
			{Tag: tag, Assets: []Asset{{Name: "a.zip"}}},
			{Tag: "v2.1.0", Assets: []Asset{{Name: "b.zip"}}},
			{Tag: "v2.9.0", Draft: true, Assets: []Asset{{Name: "c.zip"}}},
			{Tag: "v3.0.0-beta", Prerelease: true, Assets: []Asset{{Name: "d.zip"}}},
		}
		r, assets, err := Select(all, version, prerelease, nil)
		switch {
		case err == nil:
			if r.Draft {
				t.Fatal("picked a draft")
			}
			if len(assets) == 0 {
				t.Fatal("success with no assets")
			}
		case errors.Is(err, ErrNoRelease):
			if r.Tag != "" {
				t.Fatalf("ErrNoRelease with tag %q", r.Tag)
			}
		case errors.Is(err, ErrNoAsset):
		default:
			t.Fatalf("unexpected %v", err)
		}
	})
}

func FuzzVerifyDigest(f *testing.F) {
	sum := sha256.Sum256([]byte("data"))
	good := "sha256:" + hex.EncodeToString(sum[:])
	f.Add(good)
	f.Add("")
	f.Add("md5:00")
	f.Add("sha256:00")
	f.Add("SHA256:" + hex.EncodeToString(sum[:]))
	f.Add("shA256:")
	f.Fuzz(func(t *testing.T, digest string) {
		if len(digest) > 1024 {
			digest = digest[:1024]
		}
		p := filepath.Join(t.TempDir(), "f")
		if err := os.WriteFile(p, []byte("data"), 0o600); err != nil {
			t.Fatal(err)
		}
		err := (Asset{Digest: digest}).VerifyDigest(p)
		if err != nil && !errors.Is(err, ErrDigestMismatch) {
			t.Fatalf("%q: %v", digest, err)
		}
	})
}

func FuzzAuthToken(f *testing.F) {
	f.Add([]byte("gho_xxx\n"))
	f.Add([]byte("  tok  "))
	f.Add([]byte(""))
	f.Add([]byte("\n\n"))
	f.Fuzz(func(t *testing.T, out []byte) {
		if len(out) > 8<<10 {
			out = out[:8<<10]
		}
		tok := strings.TrimSpace(string(out))
		a := &Auth{Run: func(context.Context) ([]byte, error) { return out, nil }}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://api.github.com/x", nil)
		if err != nil {
			t.Fatal(err)
		}
		a.Apply(req)
		got := req.Header.Get("Authorization")
		if tok == "" {
			if got != "" {
				t.Fatalf("empty token sent as %q", got)
			}
		} else if got != "Bearer "+tok {
			t.Fatalf("Authorization=%q", got)
		}
		off, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "https://evil.example/x", nil)
		if err != nil {
			t.Fatal(err)
		}
		a.Apply(off)
		if off.Header.Get("Authorization") != "" {
			t.Fatal("token sent off GitHub")
		}
		plain, err := http.NewRequestWithContext(t.Context(), http.MethodGet, "http://api.github.com/x", nil)
		if err != nil {
			t.Fatal(err)
		}
		a.Apply(plain)
		if plain.Header.Get("Authorization") != "" {
			t.Fatal("token sent over http")
		}
	})
}

func FuzzRateLimited(f *testing.F) {
	f.Add(403, "0", "", "x")
	f.Add(429, "4000", "60", "")
	f.Add(200, "5000", "", "")
	f.Add(403, "7", "", "You have exceeded a secondary rate limit. Please wait.")
	f.Add(403, "7", "", `{"message":"Forbidden"}`)
	f.Add(429, "x", "Wed, 21 Oct 2015 07:28:00 GMT", "")
	f.Fuzz(func(t *testing.T, status int, remaining, retryAfter, body string) {
		if len(body) > 8<<10 {
			body = body[:8<<10]
		}
		if len(remaining) > 256 {
			remaining = remaining[:256]
		}
		if len(retryAfter) > 256 {
			retryAfter = retryAfter[:256]
		}
		h := make(http.Header)
		h.Set("X-Ratelimit-Remaining", remaining)
		h.Set("Retry-After", retryAfter)
		h.Set("X-Ratelimit-Reset", "1700000000")
		resp := &http.Response{StatusCode: status, Header: h, Body: io.NopCloser(strings.NewReader(body))}
		err := RateLimited(resp)
		if status != http.StatusForbidden && status != http.StatusTooManyRequests {
			if err != nil {
				t.Fatalf("status %d: %v", status, err)
			}
			return
		}
		if err == nil {
			return
		}
		if !errors.Is(err, httpkit.ErrRateLimited) {
			t.Fatalf("status %d: %v", status, err)
		}
	})
}
