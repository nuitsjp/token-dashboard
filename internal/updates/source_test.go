package updates

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"token-monitor-turzx/internal/appstate"
)

func sourceTestClient(t *testing.T, servers ...*httptest.Server) *http.Client {
	t.Helper()
	roots := x509.NewCertPool()
	for _, server := range servers {
		roots.AddCert(server.Certificate())
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.TLSClientConfig = &tls.Config{RootCAs: roots}
	t.Cleanup(transport.CloseIdleConnections)
	client := newHTTPClient()
	client.Transport = transport
	return client
}

func TestReleaseChangeDoesNotChangeInstallerSource(t *testing.T) {
	cfg, key, m := fixture(t, "0.2.0")
	installer, err := os.ReadFile(filepath.Join(cfg.Source, m.Filename))
	if err != nil {
		t.Fatal(err)
	}
	m.Filename = "app-0.2.0-amd64-setup.exe"
	manifest, err := Sign(m, key)
	if err != nil {
		t.Fatal(err)
	}
	assets := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/objects/metadata-blob":
			w.Write(manifest)
		case "/objects/installer-blob":
			w.Write(installer)
		default:
			http.NotFound(w, r)
		}
	}))
	defer assets.Close()
	const releases = "/owner/project/releases"
	var latest atomic.Value
	latest.Store("v0.2.0")
	var pinnedInstallerRequests, latestInstallerRequests atomic.Int32
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case releases + "/latest/download/update.json":
			http.Redirect(w, r, releases+"/download/"+latest.Load().(string)+"/update.json", http.StatusFound)
		case releases + "/download/v0.2.0/update.json":
			http.Redirect(w, r, assets.URL+"/objects/metadata-blob?signature=manifest-only", http.StatusFound)
		case releases + "/download/v0.2.0/" + m.Filename:
			pinnedInstallerRequests.Add(1)
			http.Redirect(w, r, assets.URL+"/objects/installer-blob?signature=installer-only", http.StatusFound)
		case releases + "/latest/download/" + m.Filename:
			latestInstallerRequests.Add(1)
			http.Redirect(w, r, releases+"/download/"+latest.Load().(string)+"/"+m.Filename, http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer origin.Close()
	cfg.Source = origin.URL + releases + "/latest/download/update.json"
	s := testUpdater(cfg, &appstate.State{}, nil, func() {})
	s.client = sourceTestClient(t, origin, assets)
	if _, err := s.check(context.Background()); err != nil {
		t.Fatal(err)
	}
	// A new release is published after this manifest was verified. Its release
	// does not contain the old version's installer filename.
	latest.Store("v0.3.0")
	status, err := s.download(context.Background())
	if err != nil || status.Phase != PhaseReady || status.Version != "0.2.0" {
		t.Fatalf("download after release change = %+v, %v", status, err)
	}
	if pinnedInstallerRequests.Load() != 1 || latestInstallerRequests.Load() != 0 {
		t.Fatalf("installer requests: fixed release = %d, latest = %d", pinnedInstallerRequests.Load(), latestInstallerRequests.Load())
	}
	staged, err := os.ReadFile(s.staged)
	if err != nil || !bytes.Equal(staged, installer) {
		t.Fatalf("staged installer differs from the verified release: %v", err)
	}
}

func TestFixedHTTPSManifestKeepsItsConfiguredDirectory(t *testing.T) {
	for _, path := range []string{"/updates/update.json", "/owner/project/releases/download/v0.2.0/update.json"} {
		t.Run(path, func(t *testing.T) {
			installerPath := strings.TrimSuffix(path, ManifestName) + "setup.exe"
			origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case path:
					http.Redirect(w, r, "/blobs/manifest", http.StatusFound)
				case "/blobs/manifest":
					io.WriteString(w, "metadata")
				case installerPath:
					io.WriteString(w, "installer")
				default:
					http.NotFound(w, r)
				}
			}))
			defer origin.Close()
			client := sourceTestClient(t, origin)
			reader, source, err := openSource(context.Background(), client, origin.URL+path, ManifestName)
			if err != nil {
				t.Fatal(err)
			}
			reader.Close()
			reader, _, err = openSource(context.Background(), client, source, "setup.exe")
			if err != nil {
				t.Fatal(err)
			}
			defer reader.Close()
			data, err := io.ReadAll(reader)
			if err != nil || string(data) != "installer" {
				t.Fatalf("installer = %q, %v", data, err)
			}
		})
	}
}

func TestLatestManifestWithoutFixedReleaseIsRejected(t *testing.T) {
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/owner/project/releases/latest/download/update.json" {
			http.Redirect(w, r, "/blobs/manifest", http.StatusFound)
			return
		}
		io.WriteString(w, "metadata")
	}))
	defer origin.Close()
	_, _, err := openSource(context.Background(), sourceTestClient(t, origin), origin.URL+"/owner/project/releases/latest/download/update.json", ManifestName)
	if err == nil || !strings.Contains(err.Error(), "fixed release URL") {
		t.Fatalf("latest without a fixed release = %v", err)
	}
}

func TestHTTPSourceAndRedirectAreRejected(t *testing.T) {
	var insecureRequests atomic.Int32
	insecure := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		insecureRequests.Add(1)
		io.WriteString(w, "insecure")
	}))
	defer insecure.Close()
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, insecure.URL+"/update.json", http.StatusFound)
	}))
	defer origin.Close()
	client := sourceTestClient(t, origin)
	for _, source := range []string{insecure.URL + "/update.json", origin.URL + "/update.json"} {
		if _, _, err := openSource(context.Background(), client, source, ManifestName); err == nil {
			t.Fatalf("HTTP source or redirect accepted: %s", source)
		}
	}
	if insecureRequests.Load() != 0 {
		t.Fatal("an HTTP request was sent")
	}
}

func TestUpdateRedirectLimitIsPreserved(t *testing.T) {
	var requests atomic.Int32
	origin := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Redirect(w, r, "/update.json", http.StatusFound)
	}))
	defer origin.Close()
	_, _, err := openSource(context.Background(), sourceTestClient(t, origin), origin.URL+"/update.json", ManifestName)
	if err == nil || !strings.Contains(err.Error(), "too many update redirects") || requests.Load() != 10 {
		t.Fatalf("redirect loop = %v after %d requests", err, requests.Load())
	}
}
