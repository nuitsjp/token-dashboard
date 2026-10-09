package display

import (
	"bytes"
	"context"
	"encoding/base64"
	"image/png"
	"log/slog"
	"sync"
	"time"

	"token-monitor-turzx/internal/fault"
	"token-monitor-turzx/internal/usage"
)

// Updated is emitted after a new image is available from Preview.
const Updated = "display:updated"

// Service coordinates rendering and publishes the latest PNG to the preview.
type Service struct {
	// State is where Limits reads the contracts from; SetShown asks it for a redraw.
	State *usage.State
	// Hidden returns the saved keys of the windows that are not drawn, and Show saves the change.
	Hidden      func() ([]string, error)
	Show        func(keys []string, shown bool) error
	Content     func() (map[string]ServiceContent, error)
	SaveContent func(provider string, enabled, showLimits, showTokens bool) error
	Logger      *slog.Logger
	Options     func() (Options, error)

	mu        sync.Mutex
	preview   string
	selection *serviceSelection
	nextFrame uint64
	pending   *pendingFrame
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
func Run(ctx context.Context, s *Service, renderer *CanvasRenderer, compactRenderer *Renderer, state *usage.State, redraw time.Duration, style func() Style, output func(Frame), emit func(string, any), logger *slog.Logger) {
	var cycle rotation
	var last Options
	var nextDraw time.Time
	dirty := true
	for {
		stats, source := state.Snapshot()
		// A selection that cannot be read draws every window, as style does for Gauges.
		hidden, err := s.hiddenSet()
		if err != nil {
			logger.Warn("hidden_limits_unavailable", "cause", err)
		}
		now := time.Now()
		options := Options{Style: style()}
		if s.Options != nil {
			options, err = s.Options()
			if err != nil {
				logger.Warn("display_options_unavailable", "cause", err)
				select {
				case <-ctx.Done():
					return
				case <-state.Changed():
					dirty = true
				case <-time.After(time.Second):
				}
				continue
			}
		}
		// Detect reconnection without repeatedly redrawing or restarting page deadlines.
		if !dirty && options == last && now.Before(nextDraw) {
			select {
			case <-ctx.Done():
				return
			case <-state.Changed():
				dirty = true
			case <-time.After(min(time.Second, time.Until(nextDraw))):
			}
			continue
		}
		visible := withoutHidden(stats, hidden)
		var frame Frame
		var renderErr error
		wait := redraw
		renderOptions := options
		renderOptions.source = source
		// Automatic can temporarily have no connected device. Continue the compact
		// cycle so reconnecting it resumes the current page, not the first page.
		if options.DeviceID == "" && cycle.options.Compact {
			renderOptions.Compact = true
			renderOptions.DeviceID = cycle.options.DeviceID
		}
		if renderOptions.Compact {
			pages, message := compactPages(visible, renderOptions)
			cycle.update(pages, renderOptions, now)
			var page *compactPage
			if len(pages) > 0 {
				page = &pages[cycle.index]
				wait = min(wait, time.Until(cycle.deadline))
			}
			frame.Image = compactRenderer.renderCompact(visible, source, renderOptions, page, cycle.index+1, len(pages), message, now)
			var buf bytes.Buffer
			renderErr = png.Encode(&buf, frame.Image)
			frame.PNG = buf.Bytes()
		} else {
			cycle = rotation{}
			frame, renderErr = renderer.Render(ctx, visible, now, source, options.Style)
			if ctx.Err() != nil {
				return
			}
		}
		frame.Target = options
		frame.Target.Compact = renderOptions.Compact
		if renderErr != nil {
			logger.Error("display_render_failed", "cause", renderErr)
		} else {
			output(frame)
			s.mu.Lock()
			s.preview = "data:image/png;base64," + base64.StdEncoding.EncodeToString(frame.PNG)
			s.mu.Unlock()
			emit(Updated, nil)
		}
		last, nextDraw, dirty = options, now.Add(wait), false
		timer := time.NewTimer(max(time.Millisecond, min(time.Second, wait)))
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-state.Changed():
			dirty = true
		case <-timer.C:
		}
		timer.Stop()
	}
}
