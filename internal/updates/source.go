package updates

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func newHTTPClient() *http.Client {
	return &http.Client{Timeout: 5 * time.Minute, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return errors.New("too many update redirects")
		}
		if req.URL.Scheme != "https" {
			return errors.New("updates require HTTPS")
		}
		return nil
	}}
}

// source is either an absolute folder (including Windows UNC) or an HTTPS URL
// for update.json. The installer must be in the same folder/release.
func openSource(ctx context.Context, client *http.Client, source, filename string) (io.ReadCloser, error) {
	if strings.HasPrefix(source, "https://") {
		base, err := url.Parse(source)
		if err != nil {
			return nil, err
		}
		if base.User != nil || base.Host == "" {
			return nil, errors.New("invalid update URL")
		}
		target := base
		if filename != ManifestName {
			target = base.ResolveReference(&url.URL{Path: filename})
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
		if err != nil {
			return nil, err
		}
		response, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return nil, errors.New("update source returned HTTP " + response.Status)
		}
		return response.Body, nil
	}
	if !filepath.IsAbs(source) {
		return nil, errors.New("update source must be an absolute folder or HTTPS manifest URL")
	}
	return os.Open(filepath.Join(source, filename))
}
