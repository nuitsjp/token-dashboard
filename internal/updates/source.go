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
// The returned source pins a latest-release manifest to the release that served it.
func openSource(ctx context.Context, client *http.Client, source, filename string) (io.ReadCloser, string, error) {
	if strings.HasPrefix(source, "https://") {
		base, err := url.Parse(source)
		if err != nil {
			return nil, "", err
		}
		if base.User != nil || base.Host == "" {
			return nil, "", errors.New("invalid update URL")
		}
		target := base
		if filename != ManifestName {
			target = base.ResolveReference(&url.URL{Path: filename})
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, target.String(), nil)
		if err != nil {
			return nil, "", err
		}
		response, err := client.Do(req)
		if err != nil {
			return nil, "", err
		}
		if response.StatusCode != http.StatusOK {
			response.Body.Close()
			return nil, "", errors.New("update source returned HTTP " + response.Status)
		}
		if filename == ManifestName {
			source, err = resolvedManifestSource(base, response)
			if err != nil {
				response.Body.Close()
				return nil, "", err
			}
		}
		return response.Body, source, nil
	}
	if !filepath.IsAbs(source) {
		return nil, "", errors.New("update source must be an absolute folder or HTTPS manifest URL")
	}
	file, err := os.Open(filepath.Join(source, filename))
	return file, source, err
}

func resolvedManifestSource(base *url.URL, response *http.Response) (string, error) {
	latest := "/releases/latest/download/" + ManifestName
	if !strings.HasSuffix(base.Path, latest) {
		return base.String(), nil
	}
	prefix := strings.TrimSuffix(base.Path, latest) + "/releases/download/"
	suffix := "/" + ManifestName
	// GitHub redirects the release URL again to an expiring asset URL. Walk
	// back through the requests to retain the release URL, not that final blob.
	for request := response.Request; request != nil; {
		candidate := request.URL
		if candidate.Scheme == base.Scheme && candidate.Host == base.Host &&
			strings.HasPrefix(candidate.Path, prefix) && strings.HasSuffix(candidate.Path, suffix) &&
			len(candidate.Path) > len(prefix)+len(suffix) {
			return candidate.String(), nil
		}
		if request.Response == nil {
			break
		}
		request = request.Response.Request
	}
	return "", errors.New("update source did not resolve to a fixed release URL")
}
