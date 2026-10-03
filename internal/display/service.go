package display

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"log/slog"
	"sync"
	"time"

	"token-monitor-turzx/internal/fault"
	"token-monitor-turzx/internal/usage"
)

// Updated is emitted after a new image is available from Preview.
const Updated = "display:updated"

// Service hands the latest image to the window. The window never draws it.
type Service struct {
	// State is where Limits reads the contracts from; SetShown asks it for a redraw.
	State *usage.State
	// Hidden returns the saved keys of the windows that are not drawn, and Show saves the change.
	Hidden func() ([]string, error)
	Show   func(keys []string, shown bool) error
	Logger *slog.Logger

	mu      sync.Mutex
	preview string
}

// hiddenSet is the saved hidden windows as a set.
func (s *Service) hiddenSet() (map[string]bool, error) {
	keys, err := s.Hidden()
	set := map[string]bool{}
	for _, k := range keys {
		set[k] = true
	}
	return set, err
}

// Limits lists the contracts and windows that can be drawn, with whether each is shown. It is empty
// until the first usage arrives.
func (s *Service) Limits() (list []LimitContract, err error) {
	defer func() { err = fault.Boundary(s.Logger, "display.limits", err) }()
	hidden, err := s.hiddenSet()
	if err != nil {
		return nil, err
	}
	stats, _ := s.State.Snapshot()
	return contractsOf(stats, hidden), nil
}

// SetShown saves that the windows with the given keys are shown or hidden and redraws at once. The
// other windows keep their state. When saving fails nothing changes. It returns the list as it is now.
func (s *Service) SetShown(keys []string, shown bool) (list []LimitContract, err error) {
	defer func() { err = fault.Boundary(s.Logger, "display.setShown", err) }()
	if err := s.Show(keys, shown); err != nil {
		return nil, err
	}
	s.State.Touch()
	return s.Limits()
}

// Preview returns the latest image as a PNG data URL, or "" before the first image.
func (s *Service) Preview() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.preview
}

// Run redraws when the state changes and at least every redraw (a minute in the app) so the time
// until reset stays current. Each image goes to the preview in s and to output. It is a function,
// not a method, so Wails does not bind it.
func Run(ctx context.Context, s *Service, renderer *Renderer, state *usage.State, redraw time.Duration, style func() Style, output func(*image.RGBA), emit func(string, any), logger *slog.Logger) {
	ticker := time.NewTicker(redraw)
	defer ticker.Stop()
	for {
		stats, source := state.Snapshot()
		// A selection that cannot be read draws every window, as style does for Gauges.
		hidden, err := s.hiddenSet()
		if err != nil {
			logger.Warn("hidden_limits_unavailable", "cause", err)
		}
		img := renderer.Render(withoutHidden(stats, hidden), time.Now(), source, style())
		output(img)
		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			logger.Error("preview_encode_failed", "cause", err)
		} else {
			s.mu.Lock()
			s.preview = "data:image/png;base64," + base64.StdEncoding.EncodeToString(buf.Bytes())
			s.mu.Unlock()
			emit(Updated, nil)
		}
		select {
		case <-ctx.Done():
			return
		case <-state.Changed():
		case <-ticker.C:
		}
	}
}
