package display

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"image"
	_ "image/jpeg"
	_ "image/png"
	"strings"
	"time"

	"token-monitor-turzx/internal/fault"
	"token-monitor-turzx/internal/usage"
)

const RenderRequested = "display:render-requested"

// FrameRequest is an immutable snapshot for the resident WebView2 renderer.
type FrameRequest struct {
	ID    uint64    `json:"id"`
	Theme string    `json:"theme"`
	Data  ThemeData `json:"data"`
}

// Frame contains two encodings of the same Canvas drawing.
type Frame struct {
	PNG  []byte
	JPEG []byte
}

type frameResult struct {
	frame Frame
	err   error
}

type pendingFrame struct {
	request FrameRequest
	result  chan frameResult
}

// CanvasRenderer asks the existing WebView2 to draw one frame at a time.
type CanvasRenderer struct {
	service         *Service
	emit            func(string, any)
	responseTimeout time.Duration
}

func NewCanvasRenderer(service *Service, emit func(string, any)) *CanvasRenderer {
	return &CanvasRenderer{service: service, emit: emit, responseTimeout: 10 * time.Second}
}

func (r *CanvasRenderer) Render(ctx context.Context, stats *usage.Stats, now time.Time, source string, style Style) (Frame, error) {
	waitCtx, cancel := context.WithTimeout(ctx, r.responseTimeout)
	defer cancel()
	s := r.service
	s.mu.Lock()
	s.nextFrame++
	pending := &pendingFrame{
		request: FrameRequest{ID: s.nextFrame, Theme: strings.ToLower(string(style)), Data: themeData(stats, now, source)},
		result:  make(chan frameResult, 1),
	}
	s.pending = pending
	s.mu.Unlock()
	defer func() {
		s.mu.Lock()
		if s.pending == pending {
			s.pending = nil
		}
		s.mu.Unlock()
	}()
	r.emit(RenderRequested, nil)
	select {
	case <-waitCtx.Done():
		if ctx.Err() != nil {
			return Frame{}, ctx.Err()
		}
		return Frame{}, fmt.Errorf("canvas rendering: response timed out: %w", waitCtx.Err())
	case result := <-pending.result:
		return result.frame, result.err
	}
}

// RenderRequest also supplies the initial request when the page first loads or reloads.
func (s *Service) RenderRequest() *FrameRequest {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.pending == nil {
		return nil
	}
	return &s.pending.request
}

// AgentIcons supplies one shared collection; themes reference these paths.
func (s *Service) AgentIcons() (icons map[string]string, err error) {
	defer func() { err = fault.Boundary(s.Logger, "display.agentIcons", err) }()
	entries, err := iconFiles.ReadDir("icons")
	if err != nil {
		return nil, err
	}
	icons = make(map[string]string, len(entries))
	for _, entry := range entries {
		data, err := iconFiles.ReadFile("icons/" + entry.Name())
		if err != nil {
			return nil, err
		}
		icons["/assets/agents/"+entry.Name()] = "data:image/png;base64," + base64.StdEncoding.EncodeToString(data)
	}
	return icons, nil
}

// CompleteFrame validates the image boundary before publishing to either output.
// A result from a previous page or an already completed request is discarded.
func (s *Service) CompleteFrame(id uint64, pngBase64, jpegBase64, renderError string) (err error) {
	defer func() { err = fault.Boundary(s.Logger, "display.completeFrame", err) }()
	s.mu.Lock()
	pending := s.pending
	s.mu.Unlock()
	if pending == nil || pending.request.ID != id {
		return nil
	}
	var result frameResult
	if renderError != "" {
		result.err = fmt.Errorf("canvas rendering: %s", renderError)
	} else {
		result.frame.PNG, err = decodeFrame(pngBase64, "png", Width, Height)
		if err == nil {
			result.frame.JPEG, err = decodeFrame(jpegBase64, "jpeg", Height, Width)
		}
		result.err = err
	}
	select {
	case pending.result <- result:
	default:
	}
	return err
}

func decodeFrame(encoded, format string, width, height int) ([]byte, error) {
	data, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, fmt.Errorf("decode %s: %w", format, err)
	}
	config, actual, err := image.DecodeConfig(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("read %s dimensions: %w", format, err)
	}
	if actual != format || config.Width != width || config.Height != height {
		return nil, fmt.Errorf("unexpected %s frame: %s %dx%d", format, actual, config.Width, config.Height)
	}
	return data, nil
}
