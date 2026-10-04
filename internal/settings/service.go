// Package settings owns the usage source, Hub connection, and TURZX display selection.
package settings

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"maps"
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
	Source   string `json:"source"`
	URL      string `json:"url"`
	TokenSet bool   `json:"tokenSet"`
	// DisplayID is empty when the first connected display is used automatically.
	DisplayID string `json:"displayID"`
	// LimitStyle is how Usage Limits are drawn: Gauges or Bars.
	LimitStyle string `json:"limitStyle"`
}

type ConnectionRequest struct {
	Source string `json:"source"`
	URL    string `json:"url"`
	// An empty token keeps the saved one.
	Token string `json:"token"`
}

// file is the on-disk format described in docs/design/data.md.
type file struct {
	Source      string `json:"source"`
	Connection  string `json:"connection,omitempty"`
	DisplayID   string `json:"displayID"`
	DisplayName string `json:"displayName"`
	// LimitStyle is Gauges or Bars; absent means Gauges.
	LimitStyle string `json:"limitStyle,omitempty"`
	// HiddenLimits are the keys of the windows that are not drawn; absent means all are drawn.
	HiddenLimits []string `json:"hiddenLimits,omitempty"`
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
	// OnSaved hands the new settings to the processes that depend on them. It must not block.
	OnSaved func()
	// OnStyleSaved tells the display that the style changed. It must not block.
	OnStyleSaved func()
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
	return viewOf(saved, conn), nil
}

// GetDisplays enumerates devices separately from reading the saved settings.
func (s *Service) GetDisplays() (displays []Display, err error) {
	defer func() { err = fault.Boundary(s.logger, "settings.displays", err) }()
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, _, err := s.read()
	if err != nil {
		return nil, err
	}
	devices, err := s.list()
	if err != nil {
		return nil, err
	}
	displays = make([]Display, 0, len(devices)+1)
	for _, d := range devices {
		displays = append(displays, Display{DeviceID: d.ID, Name: d.Name, Connected: true})
	}
	// A selected display stays selectable while it is unplugged.
	if saved.DisplayID != "" && !slices.ContainsFunc(devices, func(d turzx.Device) bool { return d.ID == saved.DisplayID }) {
		displays = append(displays, Display{DeviceID: saved.DisplayID, Name: saved.DisplayName})
	}
	return displays, nil
}

// styleOf is the saved style: Bars when saved so, otherwise Gauges.
func styleOf(saved file) string {
	if saved.LimitStyle == "Bars" {
		return "Bars"
	}
	return "Gauges"
}

func (s *Service) SaveConnection(req ConnectionRequest) (view View, err error) {
	defer func() { err = fault.Boundary(s.logger, "settings.saveConnection", err) }()
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, conn, err := s.read()
	if err != nil {
		return View{}, err
	}
	before := conn
	fields := map[string]string{}
	if req.Source != "Local" && req.Source != "Hub" {
		fields["source"] = "Choose Local or Hub."
	}
	var origin, token string
	if req.Source == "Hub" {
		var ok bool
		origin, ok = parseOrigin(req.URL)
		if !ok {
			fields["url"] = "Enter an http:// or https:// URL with a host and an optional port."
		}
		token = strings.TrimSpace(req.Token)
		if token == "" {
			token = conn.Token
		}
		if token == "" {
			fields["token"] = "Enter the access token."
		} else if strings.ContainsFunc(token, unicode.IsControl) {
			fields["token"] = "The access token must not contain control characters."
		}
	}
	if len(fields) > 0 {
		return View{}, fault.Validation(fields)
	}
	next := saved
	next.Source = req.Source
	if req.Source == "Hub" {
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
	}
	if err := s.write(next); err != nil {
		return View{}, err
	}
	s.logger.Info("settings_saved")
	// The source is read again only when the source or the connection changed. A new display or style
	// must not interrupt reading, which would blank the image until the next usage arrives.
	if s.OnSaved != nil && (next.Source != saved.Source || (next.Source == "Hub" && conn != before)) {
		s.OnSaved()
	}
	return viewOf(next, conn), nil
}

func (s *Service) SetDisplay(displayID string) (selected string, err error) {
	defer func() { err = fault.Boundary(s.logger, "settings.setDisplay", err) }()
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, _, err := s.read()
	if err != nil {
		return "", err
	}
	name := ""
	if displayID != "" {
		devices, err := s.list()
		if err != nil {
			return "", err
		}
		if i := slices.IndexFunc(devices, func(d turzx.Device) bool { return d.ID == displayID }); i >= 0 {
			name = devices[i].Name
		} else if displayID == saved.DisplayID {
			name = saved.DisplayName
		} else {
			return "", fault.Validation(map[string]string{"displayID": "The selected display is not available."})
		}
	}
	saved.DisplayID, saved.DisplayName = displayID, name
	if err := s.write(saved); err != nil {
		return "", err
	}
	s.logger.Info("settings_saved")
	return displayID, nil
}

func (s *Service) SetLimitStyle(style string) (selected string, err error) {
	defer func() { err = fault.Boundary(s.logger, "settings.setLimitStyle", err) }()
	if style != "Gauges" && style != "Bars" {
		return "", fault.Validation(map[string]string{"limitStyle": "Choose Gauges or Bars."})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, _, err := s.read()
	if err != nil {
		return "", err
	}
	before := styleOf(saved)
	saved.LimitStyle = style
	if err := s.write(saved); err != nil {
		return "", err
	}
	s.logger.Info("settings_saved")
	if before != style && s.OnStyleSaved != nil {
		s.OnStyleSaved()
	}
	return style, nil
}

func (s *Service) read() (file, connection, error) {
	var saved file
	var conn connection
	data, err := os.ReadFile(s.path)
	if errors.Is(err, fs.ErrNotExist) {
		return file{Source: "Local"}, conn, nil
	}
	if err != nil {
		return saved, conn, err
	}
	unreadable := fault.New("SETTINGS_UNREADABLE", "The saved settings cannot be read. They may belong to another Windows user.")
	if err := json.Unmarshal(data, &saved); err != nil {
		return saved, conn, unreadable
	}
	if saved.Source == "" {
		saved.Source = "Local"
	} else if saved.Source != "Local" && saved.Source != "Hub" {
		return saved, conn, unreadable
	}
	if saved.Connection == "" {
		if saved.Source == "Hub" {
			return saved, conn, unreadable
		}
		return saved, conn, nil
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

func viewOf(saved file, conn connection) View {
	return View{Source: saved.Source, URL: conn.URL, TokenSet: conn.Token != "", DisplayID: saved.DisplayID, LimitStyle: styleOf(saved)}
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

// LimitStyle returns the saved style, Gauges when none is saved or the file cannot be read. It is a
// function, not a method, so Wails does not bind it.
func LimitStyle(s *Service) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, _, err := s.read()
	if err != nil {
		return "Gauges"
	}
	return styleOf(saved)
}

// HiddenLimits returns the saved keys of the windows that are not drawn. It is a function, not a
// method, so Wails does not bind it.
func HiddenLimits(s *Service) ([]string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, _, err := s.read()
	return saved.HiddenLimits, err
}

// SetLimitsShown shows or hides the windows with the given keys. The other saved values, including the
// other windows, keep their state. It is a function, not a method, so Wails does not bind it.
func SetLimitsShown(s *Service, keys []string, shown bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, _, err := s.read()
	if err != nil {
		return err
	}
	hidden := map[string]bool{}
	for _, k := range saved.HiddenLimits {
		hidden[k] = true
	}
	for _, k := range keys {
		if shown {
			delete(hidden, k)
		} else {
			hidden[k] = true
		}
	}
	saved.HiddenLimits = slices.Sorted(maps.Keys(hidden))
	return s.write(saved)
}

// DisplayTarget returns the saved display, or the first connected one when Automatic ("" if none).
// It is a function, not a method, so Wails does not bind it.
func DisplayTarget(s *Service) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, _, err := s.read()
	if err != nil {
		return "", err
	}
	if saved.DisplayID != "" {
		return saved.DisplayID, nil
	}
	devices, err := s.list()
	if err != nil || len(devices) == 0 {
		return "", err
	}
	return devices[0].ID, nil
}

// Connection returns the saved Hub URL and token for the receiver. It is a function, not a
// method, so Wails does not bind it and the token never reaches the window.
func Connection(s *Service) (string, string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, conn, err := s.read()
	return conn.URL, conn.Token, err
}

// Source returns the saved usage source without exposing it as a Wails method.
func Source(s *Service) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, _, err := s.read()
	return saved.Source, err
}
