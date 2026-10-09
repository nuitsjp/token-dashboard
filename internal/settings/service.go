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
	DisplayID string    `json:"displayID"`
	Displays  []Display `json:"displays"`
	// LimitStyle is how Usage Limits are drawn: Gauges or Bars.
	LimitStyle              string `json:"limitStyle"`
	Orientation             string `json:"orientation"`
	RotationIntervalSeconds int    `json:"rotationIntervalSeconds"`
	SkipFull5hServices      bool   `json:"skipFull5hServices"`
}

type SaveRequest struct {
	Source string `json:"source"`
	URL    string `json:"url"`
	// An empty token keeps the saved one.
	Token     string `json:"token"`
	DisplayID string `json:"displayID"`
	// An empty LimitStyle keeps the current one.
	LimitStyle              string  `json:"limitStyle"`
	Orientation             *string `json:"orientation,omitempty"`
	RotationIntervalSeconds *int    `json:"rotationIntervalSeconds,omitempty"`
	SkipFull5hServices      *bool   `json:"skipFull5hServices,omitempty"`
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
	HiddenLimits            []string                  `json:"hiddenLimits,omitempty"`
	Orientation             string                    `json:"orientation"`
	RotationIntervalSeconds int                       `json:"rotationIntervalSeconds"`
	SkipFull5hServices      bool                      `json:"skipFull5hServices"`
	CompactServiceContent   map[string]CompactContent `json:"compactServiceContent,omitempty"`
}

// CompactContent retains absent/null flags separately from explicit false.
type CompactContent struct {
	Enabled    *bool `json:"enabled"`
	ShowLimits *bool `json:"showLimits"`
	ShowTokens *bool `json:"showTokens"`
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
	// OnDisplaySaved tells the display that its destination or rotation controls changed.
	OnDisplaySaved func()
}

func New(path, appID string, list func() ([]turzx.Device, error), logger *slog.Logger) *Service {
	return &Service{path: path, entropy: []byte(appID), list: list, logger: logger}
}

// RotationSettings are the persisted compact display controls.
type RotationSettings struct {
	Orientation        string
	IntervalSeconds    int
	SkipFull5hServices bool
}

func (s *Service) view(saved file, conn connection, devices []turzx.Device) View {
	v := viewOf(saved, conn, devices)
	v.Orientation, v.RotationIntervalSeconds, v.SkipFull5hServices = saved.Orientation, saved.RotationIntervalSeconds, saved.SkipFull5hServices
	return v
}

func rotationOf(saved file) RotationSettings {
	return RotationSettings{saved.Orientation, saved.RotationIntervalSeconds, saved.SkipFull5hServices}
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
	return s.view(saved, conn, devices), nil
}

// styleOf is the saved style: Bars when saved so, otherwise Gauges.
func styleOf(saved file) string {
	if saved.LimitStyle == "Bars" {
		return "Bars"
	}
	return "Gauges"
}

func validOrientation(value string) bool {
	switch value {
	case "Landscape", "ReverseLandscape", "Portrait", "ReversePortrait":
		return true
	default:
		return false
	}
}

func (s *Service) Save(req SaveRequest) (view View, err error) {
	defer func() { err = fault.Boundary(s.logger, "settings.save", err) }()
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, conn, err := s.read()
	if err != nil {
		return View{}, err
	}
	before := conn
	devices, err := s.list()
	if err != nil {
		return View{}, err
	}
	fields := map[string]string{}
	rotation := rotationOf(saved)
	if req.Orientation != nil {
		if !validOrientation(*req.Orientation) {
			fields["orientation"] = "Choose Landscape, Landscape (180°), Portrait, or Portrait (180°)."
		} else {
			rotation.Orientation = *req.Orientation
		}
	}
	if req.RotationIntervalSeconds != nil {
		if *req.RotationIntervalSeconds < 5 || *req.RotationIntervalSeconds > 300 {
			fields["rotationIntervalSeconds"] = "Enter a whole number from 5 to 300."
		} else {
			rotation.IntervalSeconds = *req.RotationIntervalSeconds
		}
	}
	if req.SkipFull5hServices != nil {
		rotation.SkipFull5hServices = *req.SkipFull5hServices
	}
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
	if req.LimitStyle != "" && req.LimitStyle != "Gauges" && req.LimitStyle != "Bars" {
		fields["limitStyle"] = "Choose Gauges or Bars."
	}
	next := file{Source: req.Source, Connection: saved.Connection, DisplayID: req.DisplayID, LimitStyle: saved.LimitStyle, HiddenLimits: saved.HiddenLimits, CompactServiceContent: saved.CompactServiceContent}
	next.Orientation, next.RotationIntervalSeconds, next.SkipFull5hServices = rotation.Orientation, rotation.IntervalSeconds, rotation.SkipFull5hServices
	if req.LimitStyle != "" {
		next.LimitStyle = req.LimitStyle
	}
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
	rotationChanged := rotation != rotationOf(saved)
	s.logger.Info("settings_saved")
	// The source is read again only when the source or the connection changed. A new display or style
	// must not interrupt reading, which would blank the image until the next usage arrives.
	if s.OnSaved != nil && (next.Source != saved.Source || (next.Source == "Hub" && conn != before)) {
		s.OnSaved()
	}
	if styleOf(next) != styleOf(saved) && s.OnStyleSaved != nil {
		s.OnStyleSaved()
	}
	if (next.DisplayID != saved.DisplayID || rotationChanged) && s.OnDisplaySaved != nil {
		s.OnDisplaySaved()
	}
	return s.view(next, conn, devices), nil
}

func (s *Service) read() (file, connection, error) {
	saved := file{Source: "Local", Orientation: "Landscape", RotationIntervalSeconds: 10}
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
	if !validOrientation(saved.Orientation) || saved.RotationIntervalSeconds < 5 || saved.RotationIntervalSeconds > 300 {
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

func viewOf(saved file, conn connection, devices []turzx.Device) View {
	displays := make([]Display, 0, len(devices)+1)
	for _, d := range devices {
		displays = append(displays, Display{DeviceID: d.ID, Name: d.Name, Connected: true})
	}
	// A selected display stays selectable while it is unplugged.
	if saved.DisplayID != "" && !slices.ContainsFunc(devices, func(d turzx.Device) bool { return d.ID == saved.DisplayID }) {
		displays = append(displays, Display{DeviceID: saved.DisplayID, Name: saved.DisplayName})
	}
	return View{Source: saved.Source, URL: conn.URL, TokenSet: conn.Token != "", DisplayID: saved.DisplayID, Displays: displays, LimitStyle: styleOf(saved)}
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

// CompactServices reads the persisted choices without exposing another Wails method.
func CompactServices(s *Service) (map[string]CompactContent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, _, err := s.read()
	return saved.CompactServiceContent, err
}

// SetCompactService changes one service and preserves all other settings and IDs.
func SetCompactService(s *Service, provider string, enabled, showLimits, showTokens bool) error {
	if !showLimits && !showTokens {
		return fault.Validation(map[string]string{"content": "Choose Both, Limits, or Tokens."})
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, _, err := s.read()
	if err != nil {
		return err
	}
	if saved.CompactServiceContent == nil {
		saved.CompactServiceContent = map[string]CompactContent{}
	}
	saved.CompactServiceContent[provider] = CompactContent{Enabled: &enabled, ShowLimits: &showLimits, ShowTokens: &showTokens}
	return s.write(saved)
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
