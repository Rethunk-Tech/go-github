package github

import (
	"archive/zip"
	"context"
	"errors"
	"net/http"
	"path"
	"strings"

	"github.com/Rethunk-Tech/go-httpkit"
	"github.com/Rethunk-Tech/go-httpkit/download"
)

// ZipNames lists the entry names of a zip asset without downloading it, reading only its central directory through
// a few ranged requests. Any other asset, or one of unknown size, is an error. hc nil means http.DefaultClient.
func ZipNames(ctx context.Context, hc *http.Client, a Asset) ([]string, error) {
	if !strings.EqualFold(path.Ext(a.Name), ".zip") || a.Size <= 0 {
		return nil, errors.New("not a zip")
	}
	hc = httpkit.WithRedirect(hc, httpkit.NoDowngrade)
	if hc == nil {
		hc = http.DefaultClient
	}
	c := *hc
	c.Transport = applyAuth{c.Transport}
	ctx, cancel := context.WithTimeout(ctx, apiTimeout)
	defer cancel()
	zr, err := zip.NewReader(download.NewRangeReader(ctx, &c, a.URL, a.Size), a.Size)
	if err != nil {
		return nil, err
	}
	names := make([]string, len(zr.File))
	for i, f := range zr.File {
		names[i] = f.Name
	}
	return names, nil
}

type applyAuth struct{ http.RoundTripper }

func (t applyAuth) RoundTrip(req *http.Request) (*http.Response, error) {
	req = req.Clone(req.Context())
	DefaultAuth.Apply(req)
	rt := t.RoundTripper
	if rt == nil {
		rt = http.DefaultTransport
	}
	return rt.RoundTrip(req)
}
