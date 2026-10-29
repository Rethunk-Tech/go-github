package github

import (
	"context"
	"net/http"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/Rethunk-Tech/go-desktopenv/nowindow"
)

const (
	tokenTimeout = 3 * time.Second
	tokenTTL     = 5 * time.Minute
)

// Auth lends GitHub requests a token. The zero Auth sends none. A failed or empty Run also sends none. The token is
// never logged or put into an error.
type Auth struct {
	// Run prints the token; nil means no token. GHCLI is the user's `gh` login.
	Run func(ctx context.Context) ([]byte, error)

	mu      sync.Mutex
	token   string
	fetched time.Time
}

// DefaultAuth authorizes the requests FetchReleases and Client make. It sends nothing until Run is set, because a
// library should not start a program on its first request unasked.
var DefaultAuth = &Auth{}

// GHCLI prints the token of the user's `gh` login, with no console window on Windows.
func GHCLI(ctx context.Context) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "gh", "auth", "token") // #nosec G204 -- fixed arguments
	nowindow.Set(cmd)
	return cmd.Output()
}

// get caches the answer, a failure included, so a missing or logged-out gh costs one exec per tokenTTL.
func (a *Auth) get(ctx context.Context) string {
	if a.Run == nil {
		return ""
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if !a.fetched.IsZero() && time.Since(a.fetched) < tokenTTL {
		return a.token
	}
	ctx, cancel := context.WithTimeout(ctx, tokenTimeout)
	defer cancel()
	out, err := a.Run(ctx)
	a.token, a.fetched = "", time.Now()
	if err == nil {
		a.token = strings.TrimSpace(string(out))
	}
	return a.token
}

// LoggedIn reports whether requests carry a token, which raises GitHub's rate limit.
func (a *Auth) LoggedIn(ctx context.Context) bool { return a.get(ctx) != "" }

// IsGitHubHost reports whether host is one that a token may be sent to.
func IsGitHubHost(host string) bool {
	return host == "github.com" || host == "api.github.com"
}

// Apply sets the bearer token on req when it targets GitHub itself over https and a token is available. A
// redirect to another host (release asset storage) drops the header in net/http; a same-host downgrade to
// http is refused before the token is attached.
func (a *Auth) Apply(req *http.Request) {
	if req.URL.Scheme != "https" || !IsGitHubHost(req.URL.Hostname()) {
		return
	}
	if t := a.get(req.Context()); t != "" {
		req.Header.Set("Authorization", "Bearer "+t)
	}
}
