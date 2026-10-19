package display

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
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
	TargetID     string    `json:"targetID"`
	ID           uint64    `json:"id"`
	Theme        string    `json:"theme"`
	Data         ThemeData `json:"data"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	OutputFormat string    `json:"outputFormat"`
	Reverse180   bool      `json:"reverse180"`
	Generation   uint64    `json:"generation"`
	// ShiftX and ShiftY move the image sent to the TURZX (not the PNG for the preview) by that many pixels.
	ShiftX int `json:"shiftX"`
	ShiftY int `json:"shiftY"`
}

// Frame contains two encodings of the same Canvas drawing.
type Frame struct {
	Request      FrameRequest
	PNG          []byte
	JPEG         []byte
	DeviceImage  []byte
	OutputFormat string
	// Pages is how many pages the style has; the frame is one of them.
	Pages int
	// presence closes on either away transition, invalidating queued images.
	presence <-chan struct{}
	away     bool
}

var errPresenceChanged = errors.New("display presence changed")

func (f Frame) presenceCurrent() bool {
	select {
	case <-f.presence:
		return false
	default:
		return true
	}
}

type frameResult struct {
	frame Frame
	err   error
}

type pendingFrame struct {
	request  FrameRequest
	result   chan frameResult
	presence <-chan struct{}
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

func (r *CanvasRenderer) Render(ctx context.Context, stats *usage.Stats, now time.Time, source string, style Style, page int) (Frame, error) {
	data := themeData(stats, now, source, page)
	pages := data.GaugePages
	if style == Bars {
		pages = data.BarPages
	}
	r.service.mu.Lock()
	x, y := r.service.shiftX, r.service.shiftY
	r.service.mu.Unlock()
	return r.RenderPrepared(ctx, FrameRequest{Theme: strings.ToLower(string(style)), Data: data, Width: Width, Height: Height, OutputFormat: "jpeg", ShiftX: x, ShiftY: y}, pages)
}

// RenderPrepared shares the image boundary with the legacy display and style previews.
func (r *CanvasRenderer) RenderPrepared(ctx context.Context, request FrameRequest, pages int) (Frame, error) {
	waitCtx, cancel := context.WithTimeout(ctx, r.responseTimeout)
	defer cancel()
	s := r.service
	away, presence := s.awayState()
	if away {
		return Frame{}, errPresenceChanged
	}
	s.mu.Lock()
	if s.away || s.awayCh != presence {
		s.mu.Unlock()
		return Frame{}, errPresenceChanged
	}
	s.nextFrame++
	request.ID = s.nextFrame
	pending := &pendingFrame{
		request:  request,
		result:   make(chan frameResult, 1),
		presence: presence,
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
	case <-presence:
		return Frame{}, errPresenceChanged
	case <-waitCtx.Done():
		if ctx.Err() != nil {
			return Frame{}, ctx.Err()
		}
		return Frame{}, fmt.Errorf("canvas rendering: response timed out: %w", waitCtx.Err())
	case result := <-pending.result:
		frame := result.frame
		frame.Request = request
		frame.Pages = pages
		frame.presence = presence
		return frame, result.err
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
	select {
	case <-pending.presence:
		return nil
	default:
	}
	if s.Configuration != nil {
		current, configErr := s.Configuration()
		if configErr != nil || current.Generation != pending.request.Generation || current.TargetID != pending.request.TargetID {
			select {
			case pending.result <- frameResult{err: fmt.Errorf("display configuration changed")}:
			default:
			}
			return nil
		}
	}
	var result frameResult
	if renderError != "" {
		result.err = fmt.Errorf("canvas rendering: %s", renderError)
	} else {
		request := pending.request
		result.frame.PNG, err = decodeFrame(pngBase64, "png", request.Width, request.Height)
		if err == nil {
			w, h := request.Width, request.Height
			if request.OutputFormat == "jpeg" {
				w, h = h, w
			}
			result.frame.DeviceImage, err = decodeFrame(jpegBase64, request.OutputFormat, w, h)
			result.frame.OutputFormat = request.OutputFormat
			if request.OutputFormat == "jpeg" {
				result.frame.JPEG = result.frame.DeviceImage
			}
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
