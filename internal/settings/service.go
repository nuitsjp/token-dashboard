// Package settings owns the Hub connection and the TURZX display selection.
package settings

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/url"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"unicode"

	"token-monitor-turzx/internal/fault"
	"token-monitor-turzx/internal/turzx"
)

// Display is a TURZX device that can be selected as the output.
type Display struct {
	DeviceID  string `json:"deviceID"`
	Name      string `json:"name"`
	Connected bool   `json:"connected"`
}

// View never contains the token itself, only whether one is saved.
type View struct {
	URL      string `json:"url"`
	TokenSet bool   `json:"tokenSet"`
	// DisplayID is empty when the first connected display is used automatically.
	DisplayID string    `json:"displayID"`
	Displays  []Display `json:"displays"`
}

type SaveRequest struct {
	URL string `json:"url"`
	// An empty token keeps the saved one.
	Token     string `json:"token"`
	DisplayID string `json:"displayID"`
}

// file is the on-disk format described in docs/design/data.md.
type file struct {
	Connection  string `json:"connection"`
	DisplayID   string `json:"displayID"`
	DisplayName string `json:"displayName"`
}

type connection struct {
	URL   string `json:"url"`
	Token string `json:"token"`
}

type Service struct {
	mu      sync.Mutex
	path    string
	entropy []byte
	list    func() ([]turzx.Device, error)
	logger  *slog.Logger
}

func New(path, appID string, list func() ([]turzx.Device, error), logger *slog.Logger) *Service {
	return &Service{path: path, entropy: []byte(appID), list: list, logger: logger}
}

func (s *Service) Get() (view View, err error) {
	defer func() { err = fault.Boundary(s.logger, "settings.get", err) }()
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, conn, err := s.read()
	if err != nil {
		return View{}, err
	}
	devices, err := s.list()
	if err != nil {
		return View{}, err
	}
	return viewOf(saved, conn, devices), nil
}

func (s *Service) Save(req SaveRequest) (view View, err error) {
	defer func() { err = fault.Boundary(s.logger, "settings.save", err) }()
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, conn, err := s.read()
	if err != nil {
		return View{}, err
	}
	devices, err := s.list()
	if err != nil {
		return View{}, err
	}
	fields := map[string]string{}
	origin, ok := parseOrigin(req.URL)
	if !ok {
		fields["url"] = "Enter an http:// or https:// URL with a host and an optional port."
	}
	token := strings.TrimSpace(req.Token)
	if token == "" {
		token = conn.Token
	}
	if token == "" {
		fields["token"] = "Enter the access token."
	} else if strings.ContainsFunc(token, unicode.IsControl) {
		fields["token"] = "The access token must not contain control characters."
	}
	next := file{DisplayID: req.DisplayID}
	if req.DisplayID != "" {
		if i := slices.IndexFunc(devices, func(d turzx.Device) bool { return d.ID == req.DisplayID }); i >= 0 {
			next.DisplayName = devices[i].Name
		} else if req.DisplayID == saved.DisplayID {
			next.DisplayName = saved.DisplayName
		} else {
			fields["displayID"] = "The selected display is not available."
		}
	}
	if len(fields) > 0 {
		return View{}, fault.Validation(fields)
	}
	conn = connection{URL: origin, Token: token}
	plain, err := json.Marshal(conn)
	if err != nil {
		return View{}, err
	}
	sealed, err := protect(plain, s.entropy)
	if err != nil {
		return View{}, err
	}
	next.Connection = base64.StdEncoding.EncodeToString(sealed)
	if err := s.write(next); err != nil {
		return View{}, err
	}
	s.logger.Info("settings_saved")
	return viewOf(next, conn, devices), nil
}

func (s *Service) read() (file, connection, error) {
	var saved file
	var conn connection
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return saved, conn, nil
	}
	if err != nil {
		return saved, conn, err
	}
	unreadable := fault.New("SETTINGS_UNREADABLE", "The saved settings cannot be read. They may belong to another Windows user.")
	if err := json.Unmarshal(data, &saved); err != nil {
		return saved, conn, unreadable
	}
	sealed, err := base64.StdEncoding.DecodeString(saved.Connection)
	if err != nil {
		return saved, conn, unreadable
	}
	plain, err := unprotect(sealed, s.entropy)
	if err != nil {
		return saved, conn, unreadable
	}
	if err := json.Unmarshal(plain, &conn); err != nil {
		return saved, conn, unreadable
	}
	return saved, conn, nil
}

// write replaces the file only after the new content is fully on disk.
func (s *Service) write(saved file) error {
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(s.path), "settings-*.tmp")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmp.Name(), s.path)
}

func viewOf(saved file, conn connection, devices []turzx.Device) View {
	displays := make([]Display, 0, len(devices)+1)
	for _, d := range devices {
		displays = append(displays, Display{DeviceID: d.ID, Name: d.Name, Connected: true})
	}
	// A selected display stays selectable while it is unplugged.
	if saved.DisplayID != "" && !slices.ContainsFunc(devices, func(d turzx.Device) bool { return d.ID == saved.DisplayID }) {
		displays = append(displays, Display{DeviceID: saved.DisplayID, Name: saved.DisplayName})
	}
	return View{URL: conn.URL, TokenSet: conn.Token != "", DisplayID: saved.DisplayID, Displays: displays}
}

// parseOrigin accepts only http(s)://host[:port] with an optional trailing slash.
func parseOrigin(value string) (string, bool) {
	u, err := url.Parse(strings.TrimSpace(value))
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" || u.User != nil ||
		(u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" {
		return "", false
	}
	return u.Scheme + "://" + u.Host, true
}
