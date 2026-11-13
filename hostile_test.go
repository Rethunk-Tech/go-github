package github

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

type answer struct {
	code             int
	link, retryAfter string
	remaining, reset string
	body             []byte
}

// FuzzHostileAnswer holds that no status, Link, Retry-After or quota header, and no body, makes a call panic, and
// that Releases asks at most maxReleasePages times however the Link header loops. "SRV" in a Link is the fake
// server's address. One server serves the whole run: a server per input runs the machine out of ports.
func FuzzHostileAnswer(f *testing.F) {
	f.Add(200, `<SRV/repos/o/r/releases?per_page=30>; rel="next"`, "", "", "", []byte(`[{"tag_name":"v1"}]`))
	f.Add(200, `<https://evil.example/x>; rel="next"`, "", "", "", []byte(`[]`))
	f.Add(200, `<SRV>; rel="next", <>; rel="next"`, "", "", "", []byte(strings.Repeat("[", 20000)))
	f.Add(403, "", "99999999999999999999", "0", "-9223372036854775808", []byte(`secondary rate limit`))
	f.Add(429, "", "Wed, 21 Oct 2015 07:28:00 GMT", "x", "99999999999999999999", []byte(nil))
	var a atomic.Pointer[answer]
	a.Store(&answer{code: 200})
	var hits atomic.Int32
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		cur := a.Load()
		h := w.Header()
		h["Link"] = []string{strings.ReplaceAll(cur.link, "SRV", srv.URL)}
		h["Retry-After"] = []string{cur.retryAfter}
		h["X-Ratelimit-Remaining"] = []string{cur.remaining}
		h["X-Ratelimit-Reset"] = []string{cur.reset}
		w.WriteHeader(cur.code)
		_, _ = w.Write(cur.body)
	}))
	f.Cleanup(srv.Close)
	f.Fuzz(func(t *testing.T, code int, link, retryAfter, remaining, reset string, body []byte) {
		if code < 200 || code > 599 || code/100 == 3 {
			code = 200
		}
		a.Store(&answer{code: code, link: link, retryAfter: retryAfter, remaining: remaining, reset: reset, body: body})
		c := &Client{APIBase: srv.URL}
		hits.Store(0)
		_, _ = c.Releases(t.Context(), "o", "r")
		if n := hits.Load(); n > maxReleasePages {
			t.Fatalf("Releases asked %d times for Link %q", n, link)
		}
		_, _ = c.ReleaseNotes(t.Context(), "o", "r", "v1")
		_, _ = c.Readme(t.Context(), "o", "r")
		_, _ = c.Archived(t.Context(), "o", "r")
		_, _ = c.LicenseText(t.Context(), "mit")
		_ = CurrentRate()
	})
}

func TestALyingContentLengthFails(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		conn, rw, err := http.NewResponseController(w).Hijack()
		if err != nil {
			t.Error(err)
			return
		}
		defer func() { _ = conn.Close() }()
		_, _ = rw.WriteString("HTTP/1.1 200 OK\r\nContent-Length: 1099511627776\r\n\r\n[]")
		_ = rw.Flush()
	}))
	t.Cleanup(srv.Close)
	if _, err := (&Client{APIBase: srv.URL}).Releases(t.Context(), "o", "r"); err == nil {
		t.Fatal("a body cut short of its Content-Length was accepted")
	}
}
