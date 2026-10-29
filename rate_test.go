package github

import (
	"errors"
	"net/http"
	"strconv"
	"testing"
	"time"

	"github.com/Rethunk-Tech/go-httpkit"
)

func TestRateLimitedPrefersRetryAfter(t *testing.T) {
	t.Parallel()
	var e *httpkit.RateLimitError
	h := http.Header{
		"X-Ratelimit-Remaining": {"4000"}, "Retry-After": {"60"},
		"X-Ratelimit-Reset": {strconv.FormatInt(time.Now().Add(50*time.Minute).Unix(), 10)},
	}
	if !errors.As(RateLimited(&http.Response{StatusCode: http.StatusForbidden, Header: h}), &e) {
		t.Fatal("not a rate limit")
	}
	if d := time.Until(e.Reset); d < 50*time.Second || d > 60*time.Second {
		t.Fatalf("wait %v", d)
	}
	at := time.Now().Add(90 * time.Second).UTC().Truncate(time.Second)
	h = http.Header{"Retry-After": {at.Format(http.TimeFormat)}}
	if !errors.As(RateLimited(&http.Response{StatusCode: http.StatusTooManyRequests, Header: h}), &e) || !e.Reset.Equal(at) {
		t.Fatalf("date reset %v", e.Reset)
	}
}

func TestRateLimitedNeedsALimitAnswer(t *testing.T) {
	t.Parallel()
	h := http.Header{"X-Ratelimit-Remaining": {"0"}}
	if err := RateLimited(&http.Response{StatusCode: http.StatusOK, Header: h}); err != nil {
		t.Fatalf("a 200 is not a limit answer: %v", err)
	}
	if err := RateLimited(&http.Response{StatusCode: http.StatusForbidden, Header: http.Header{"X-Ratelimit-Remaining": {"7"}}}); err != nil {
		t.Fatalf("a 403 with quota left is not the limit: %v", err)
	}
	if err := RateLimited(&http.Response{StatusCode: http.StatusForbidden, Header: h}); !errors.Is(err, httpkit.ErrRateLimited) {
		t.Fatalf("a 403 with none left: %v", err)
	}
}

// Not parallel: the recorded rate is process-wide.
func TestObserveRecordsTheQuota(t *testing.T) {
	observe := func(status int, left string) error {
		h := http.Header{}
		h.Set("X-Ratelimit-Limit", "5000")
		h.Set("X-Ratelimit-Remaining", left)
		h.Set("X-Ratelimit-Reset", "1700000000")
		resp := &http.Response{StatusCode: status, Header: h, Body: http.NoBody}
		defer func() { _ = resp.Body.Close() }()
		return Observe(resp)
	}
	if err := observe(http.StatusOK, "4321"); err != nil {
		t.Fatal(err)
	}
	if r := CurrentRate(); !r.Known || r.Limit != 5000 || r.Remaining != 4321 || !r.Reset.Equal(time.Unix(1700000000, 0)) {
		t.Fatalf("rate after a 200 %+v", r)
	}
	var e *httpkit.RateLimitError
	if err := observe(http.StatusForbidden, "0"); !errors.As(err, &e) {
		t.Fatalf("a 403 limit answer: %v", err)
	}
	if r := CurrentRate(); r.Remaining != 0 {
		t.Fatalf("rate after a 403 %+v", r)
	}
}
