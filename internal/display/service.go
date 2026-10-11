package display

import (
	"context"
	"encoding/base64"
	"log/slog"
	"sync"
	"time"

	"token-monitor-turzx/internal/fault"
	"token-monitor-turzx/internal/usage"
)

// Updated is emitted after a new image is available from Preview.
const Updated = "display:updated"

// Service coordinates the resident renderer and publishes its latest PNG to the preview.
type Service struct {
	// State is where Limits reads the contracts from; SetShown asks it for a redraw.
	State *usage.State
	// Hidden returns the saved keys of the windows that are not drawn, and Show saves the change.
	Hidden func() ([]string, error)
	Show   func(keys []string, shown bool) error
	Logger *slog.Logger
	// Cycle returns whether the contracts that do not fit the first page are shown in turn, and how long
	// each page stays.
	Cycle func() (on bool, every time.Duration)
	// ShiftEvery is how long the image for the TURZX stays at one offset; zero means 3 minutes.
	ShiftEvery time.Duration

	mu        sync.Mutex
	preview   string
	nextFrame uint64
	pending   *pendingFrame
	shiftX    int
	shiftY    int
}

// shiftSteps are the offsets, in pixels, that the image for the TURZX takes in turn so that no pixel
// stays lit for long.
var shiftSteps = [...][2]int{{0, 0}, {4, 0}, {4, 4}, {0, 4}, {-4, 4}, {-4, 0}, {-4, -4}, {0, -4}}

func (s *Service) shiftEvery() time.Duration {
	if s.ShiftEvery <= 0 {
		return 3 * time.Minute
	}
	return s.ShiftEvery
}

// setShift is the offset of the image for the TURZX; Render reads it when it asks for a frame.
func (s *Service) setShift(step int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.shiftX, s.shiftY = shiftSteps[step][0], shiftSteps[step][1]
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

// cycleSetting is the saved cycling; without one the pages do not cycle.
func (s *Service) cycleSetting() (bool, time.Duration) {
	if s.Cycle == nil {
		return false, 10 * time.Second
	}
	on, every := s.Cycle()
	return on, every
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
func Run(ctx context.Context, s *Service, renderer *CanvasRenderer, state *usage.State, redraw time.Duration, style func() Style, output func([]byte), emit func(string, any), logger *slog.Logger) {
	ticker := time.NewTicker(redraw)
	defer ticker.Stop()
	cycle, every := s.cycleSetting()
	turn := time.NewTicker(every)
	defer turn.Stop()
	shifter := time.NewTicker(s.shiftEvery())
	defer shifter.Stop()
	page, step := 0, 0
	for {
		s.setShift(step)
		var d time.Duration
		if cycle, d = s.cycleSetting(); d != every {
			every = d
			turn.Reset(d)
		}
		stats, source := state.Snapshot()
		// A selection that cannot be read draws every window, as style does for Gauges.
		hidden, err := s.hiddenSet()
		if err != nil {
			logger.Warn("hidden_limits_unavailable", "cause", err)
		}
		if !cycle {
			page = 0
		}
		frame, err := renderer.Render(ctx, withoutHidden(stats, hidden), time.Now(), source, style(), page)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			logger.Error("display_render_failed", "cause", err)
		} else {
			output(frame.JPEG)
			s.mu.Lock()
			s.preview = "data:image/png;base64," + base64.StdEncoding.EncodeToString(frame.PNG)
			s.mu.Unlock()
			emit(Updated, nil)
			if page >= frame.Pages {
				page = 0
			}
		}
		pages := frame.Pages
	wait:
		for {
			select {
			case <-ctx.Done():
				return
			case <-state.Changed():
				break wait
			case <-ticker.C:
				break wait
			case <-shifter.C:
				step = (step + 1) % len(shiftSteps)
				break wait
			case <-turn.C:
				// Only a cycle over more than one page redraws on this beat.
				if cycle && pages > 1 {
					page = (page + 1) % pages
					break wait
				}
			}
		}
	}
}
