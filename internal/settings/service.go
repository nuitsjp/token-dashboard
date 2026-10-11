// Package settings owns the usage source, Hub connection, and TURZX display selection.
package settings

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
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
	LimitStyle string `json:"limitStyle"`
	// Cycle shows the contracts that do not fit the first page in turn, each page for CycleSeconds.
	Cycle        bool `json:"cycle"`
	CycleSeconds int  `json:"cycleSeconds"`
	// AwayMinutes is how long without input counts as away from the PC.
	AwayMinutes int `json:"awayMinutes"`
}

// Bounds and default of the time without input that counts as away, in minutes.
const (
	MinAwayMinutes     = 1
	MaxAwayMinutes     = 240
	DefaultAwayMinutes = 10
)

// Bounds and default of the time each page stays, in seconds.
const (
	MinCycleSeconds     = 5
	MaxCycleSeconds     = 3600
	DefaultCycleSeconds = 10
)

type SaveRequest struct {
	Source string `json:"source"`
	URL    string `json:"url"`
	// An empty token keeps the saved one.
	Token     string `json:"token"`
	DisplayID string `json:"displayID"`
	// An empty LimitStyle keeps the current one.
	LimitStyle string `json:"limitStyle"`
	// A nil Cycle and a zero CycleSeconds keep the current values.
	Cycle        *bool `json:"cycle"`
	CycleSeconds int   `json:"cycleSeconds"`
	// A zero AwayMinutes keeps the current value.
	AwayMinutes int `json:"awayMinutes"`
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
	// Cycle absent means false; CycleSeconds absent means DefaultCycleSeconds.
	Cycle        bool `json:"cycle,omitempty"`
	CycleSeconds int  `json:"cycleSeconds,omitempty"`
	// AwayMinutes absent means DefaultAwayMinutes.
	AwayMinutes int `json:"awayMinutes,omitempty"`
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
	// OnDisplaySaved tells the display that the style or the cycling changed. It must not block.
	OnDisplaySaved func()

	// awayMinutes is the saved value once it is known, so the presence watcher need not read the file
	// every second.
	awayKnown   bool
	awayMinutes int
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

// styleOf is the saved style: Bars when saved so, otherwise Gauges.
func styleOf(saved file) string {
	if saved.LimitStyle == "Bars" {
		return "Bars"
	}
	return "Gauges"
}

// cycleSecondsOf is the saved time each page stays; a missing or out-of-range value means the default.
func cycleSecondsOf(saved file) int {
	if saved.CycleSeconds < MinCycleSeconds || saved.CycleSeconds > MaxCycleSeconds {
		return DefaultCycleSeconds
	}
	return saved.CycleSeconds
}

// awayMinutesOf is the saved time without input that counts as away; a missing or out-of-range value
// means the default.
func awayMinutesOf(saved file) int {
	if saved.AwayMinutes < MinAwayMinutes || saved.AwayMinutes > MaxAwayMinutes {
		return DefaultAwayMinutes
	}
	return saved.AwayMinutes
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
	if req.CycleSeconds != 0 && (req.CycleSeconds < MinCycleSeconds || req.CycleSeconds > MaxCycleSeconds) {
		fields["cycleSeconds"] = fmt.Sprintf("Enter %d to %d seconds.", MinCycleSeconds, MaxCycleSeconds)
	}
	if req.AwayMinutes != 0 && (req.AwayMinutes < MinAwayMinutes || req.AwayMinutes > MaxAwayMinutes) {
		fields["awayMinutes"] = fmt.Sprintf("Enter %d to %d minutes.", MinAwayMinutes, MaxAwayMinutes)
	}
	next := file{Source: req.Source, Connection: saved.Connection, DisplayID: req.DisplayID, LimitStyle: saved.LimitStyle, HiddenLimits: saved.HiddenLimits,
		Cycle: saved.Cycle, CycleSeconds: saved.CycleSeconds, AwayMinutes: saved.AwayMinutes}
	if req.AwayMinutes != 0 {
		next.AwayMinutes = req.AwayMinutes
	}
	if req.Cycle != nil {
		next.Cycle = *req.Cycle
	}
	if req.CycleSeconds != 0 {
		next.CycleSeconds = req.CycleSeconds
	}
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
	s.awayKnown, s.awayMinutes = true, awayMinutesOf(next)
	s.logger.Info("settings_saved")
	// The source is read again only when the source or the connection changed. A new display or style
	// must not interrupt reading, which would blank the image until the next usage arrives.
	if s.OnSaved != nil && (next.Source != saved.Source || (next.Source == "Hub" && conn != before)) {
		s.OnSaved()
	}
	if (styleOf(next) != styleOf(saved) || next.Cycle != saved.Cycle || cycleSecondsOf(next) != cycleSecondsOf(saved)) && s.OnDisplaySaved != nil {
		s.OnDisplaySaved()
	}
	return viewOf(next, conn, devices), nil
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

func viewOf(saved file, conn connection, devices []turzx.Device) View {
	displays := make([]Display, 0, len(devices)+1)
	for _, d := range devices {
		displays = append(displays, Display{DeviceID: d.ID, Name: d.Name, Connected: true})
	}
	// A selected display stays selectable while it is unplugged.
	if saved.DisplayID != "" && !slices.ContainsFunc(devices, func(d turzx.Device) bool { return d.ID == saved.DisplayID }) {
		displays = append(displays, Display{DeviceID: saved.DisplayID, Name: saved.DisplayName})
	}
	return View{Source: saved.Source, URL: conn.URL, TokenSet: conn.Token != "", DisplayID: saved.DisplayID, Displays: displays, LimitStyle: styleOf(saved), Cycle: saved.Cycle, CycleSeconds: cycleSecondsOf(saved), AwayMinutes: awayMinutesOf(saved)}
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

// CycleSetting returns whether the pages cycle and how long each stays, off and the default when the
// file cannot be read. It is a function, not a method, so Wails does not bind it.
func CycleSetting(s *Service) (bool, int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	saved, _, err := s.read()
	if err != nil {
		return false, DefaultCycleSeconds
	}
	return saved.Cycle, cycleSecondsOf(saved)
}

// AwayMinutesSetting returns how many minutes without input count as away, the default when the file
// cannot be read. It is a function, not a method, so Wails does not bind it.
func AwayMinutesSetting(s *Service) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.awayKnown {
		saved, _, err := s.read()
		if err != nil {
			return DefaultAwayMinutes
		}
		s.awayKnown, s.awayMinutes = true, awayMinutesOf(saved)
	}
	return s.awayMinutes
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
