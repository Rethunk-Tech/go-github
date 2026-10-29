package github

import (
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Rethunk-Tech/go-httpkit"
)

// Rate is the quota GitHub last reported on a response.
type Rate struct {
	Known     bool      `json:"known"`
	Limit     int       `json:"limit"`
	Remaining int       `json:"remaining"`
	Reset     time.Time `json:"reset"`
}

// The quota belongs to the token or IP address, not to a Client, so it is kept once per process.
var (
	rateMu   sync.Mutex
	lastRate Rate
)

// CurrentRate is the quota from the most recent GitHub response Observe saw that carried one.
func CurrentRate() Rate {
	rateMu.Lock()
	defer rateMu.Unlock()
	return lastRate
}

func noteRate(resp *http.Response) {
	limit, e1 := strconv.Atoi(resp.Header.Get("X-Ratelimit-Limit"))
	left, e2 := strconv.Atoi(resp.Header.Get("X-Ratelimit-Remaining"))
	reset, e3 := strconv.ParseInt(resp.Header.Get("X-Ratelimit-Reset"), 10, 64)
	if e1 != nil || e2 != nil || e3 != nil {
		return
	}
	rateMu.Lock()
	lastRate = Rate{Known: true, Limit: limit, Remaining: left, Reset: time.Unix(reset, 0)}
	rateMu.Unlock()
}

// RateLimited returns a *httpkit.RateLimitError when resp is GitHub's limit answer, else nil: a 403 or 429 with
// Retry-After or X-Ratelimit-Remaining "0". Reset comes from Retry-After, else X-Ratelimit-Reset.
func RateLimited(resp *http.Response) error {
	if resp.StatusCode != http.StatusForbidden && resp.StatusCode != http.StatusTooManyRequests {
		return nil
	}
	if resp.Header.Get("X-Ratelimit-Remaining") != "0" && resp.Header.Get("Retry-After") == "" {
		return nil
	}
	e := &httpkit.RateLimitError{}
	if t, ok := httpkit.RetryAfter(resp.Header.Get("Retry-After")); ok {
		e.Reset = t
	} else if secs, err := strconv.ParseInt(resp.Header.Get("X-Ratelimit-Reset"), 10, 64); err == nil {
		e.Reset = time.Unix(secs, 0)
	}
	return e
}

// Observe records resp's quota for CurrentRate, then returns RateLimited(resp). It is one check function for any
// client making its own GitHub requests: it fits httpkit.DoCapped and the download package's Options.Check.
func Observe(resp *http.Response) error {
	noteRate(resp)
	return RateLimited(resp)
}
