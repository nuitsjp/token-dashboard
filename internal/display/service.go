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

	"token-monitor-turzx/internal/usage"
)

// Updated is emitted after a new image is available from Preview.
const Updated = "display:updated"

// Service hands the latest image to the window. The window never draws it.
type Service struct {
	mu      sync.Mutex
	preview string
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
		img := renderer.Render(stats, time.Now(), source, style())
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
