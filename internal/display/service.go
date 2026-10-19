package display

import (
	"context"
	"log/slog"
	"strings"
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
	ShiftEvery    time.Duration
	Themes        map[string]Theme
	Theme         func(string) (Theme, error)
	Configuration func() (Configuration, error)

	mu        sync.Mutex
	preview   string
	nextFrame uint64
	pending   *pendingFrame
	shiftX    int
	shiftY    int
	// away is whether the user is away, and awayCh wakes Run when it changes.
	away   bool
	awayCh chan struct{}
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

// Run retains the existing large-display entry point and uses the shared scheduler.
func Run(ctx context.Context, s *Service, renderer *CanvasRenderer, state *usage.State, redraw time.Duration, style func() Style, output func([]byte), emit func(string, any), logger *slog.Logger) {
	s.State = state
	s.Logger = logger
	configuration := func() (Configuration, error) {
		_, source := state.Snapshot()
		cycle, every := s.cycleSetting()
		selected := style()
		return Configuration{Theme: Theme{ID: strings.ToLower(string(selected)), Width: Width, Height: Height, DataLayout: strings.ToLower(string(selected))}, Source: source, Cycle: cycle, CycleEvery: every}, nil
	}
	runPresentation(ctx, s, renderer, redraw, configuration, func(frame Frame) { output(frame.JPEG) }, emit)
}
