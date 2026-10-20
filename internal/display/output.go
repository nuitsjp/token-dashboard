package display

import (
	"bytes"
	"context"
	"encoding/binary"
	"fmt"
	"image"
	"image/png"
	"log/slog"
	"time"

	"token-monitor-turzx/internal/turzx"
)

// Output sends validated Canvas frames using the selected device's transport.
// Only the newest pending image is kept.
type Output struct {
	configuration func() (Configuration, error)
	Redraw        func()
	logger        *slog.Logger
	pending       chan Frame
	done          chan struct{}
}

// NewOutput rechecks the selection and settings generation before each send.
func NewOutput(configuration func() (Configuration, error), logger *slog.Logger) *Output {
	return &Output{configuration: configuration, logger: logger, pending: make(chan Frame, 1), done: make(chan struct{})}
}

// Submit replaces any image still waiting to be sent. It must be called from one goroutine.
func (o *Output) Submit(frame Frame) {
	select {
	case <-o.pending:
	default:
	}
	o.pending <- frame
}

// Run sends until ctx ends, then restarts the display so it returns to its start-up screen.
func (o *Output) Run(ctx context.Context) {
	defer close(o.done)
	var conn *turzx.Conn
	var connID, lastFailure string
	tick := time.NewTicker(2 * time.Second)
	defer tick.Stop()
	var latest *Frame
	var retryAt time.Time
	var sentGeneration uint64
	var sentPresence <-chan struct{}
	var sentShift [2]int
	fail := func(event string, err error) {
		// Log a failure once until it changes, not on every image.
		if err.Error() != lastFailure {
			lastFailure = err.Error()
			o.logger.Warn(event, "cause", err)
		}
	}
	for {
		select {
		case <-ctx.Done():
			if conn != nil {
				if err := conn.Restart(); err != nil {
					o.logger.Warn("turzx_restart_failed", "cause", err)
				}
				conn.Close()
			}
			return
		case frame := <-o.pending:
			latest = &frame
		case <-tick.C:
		}
		if latest == nil {
			config, err := o.configuration()
			if err != nil {
				fail("turzx_target_failed", err)
				continue
			}
			if conn != nil && (config.TargetID != connID || !config.TargetConnected) {
				conn.Close()
				conn = nil
			}
			profile, known := turzx.ProfileForID(config.TargetID)
			if known && config.TargetConnected && profile.Supports(config.Theme.Width, config.Theme.Height) && (conn == nil || sentGeneration != config.Generation) && o.Redraw != nil && !time.Now().Before(retryAt) {
				o.Redraw()
			}
			continue
		}
		if time.Now().Before(retryAt) {
			continue
		}
		config, err := o.configuration()
		if err != nil {
			fail("turzx_target_failed", err)
			continue
		}
		id := config.TargetID
		request := latest.Request
		if !latest.presenceCurrent() {
			latest = nil
			continue
		}
		profile, known := turzx.ProfileForID(id)
		if !known || !profile.Supports(config.Theme.Width, config.Theme.Height) || request.Generation != config.Generation || request.TargetID != id || request.Theme != config.Theme.ID || request.Width != config.Theme.Width || request.Height != config.Theme.Height {
			if conn != nil {
				conn.Close()
				conn = nil
			}
			latest = nil
			continue
		}
		if conn != nil && connID != id {
			conn.Close()
			conn = nil
		}
		if conn == nil {
			if id == "" {
				continue
			}
			if conn, err = turzx.Open(id); err != nil {
				conn = nil
				fail("turzx_open_failed", err)
				latest = nil
				retryAt = time.Now().Add(2 * time.Second)
				continue
			}
			connID = id
			sentGeneration = 0
			sentPresence = nil
		}
		current, err := o.configuration()
		if err != nil || current.Generation != request.Generation || current.TargetID != id || !latest.presenceCurrent() {
			latest = nil
			continue
		}
		// A failed image is dropped; the next image reopens the device.
		if profile.Transport == "winusb" && latest.OutputFormat == "jpeg" {
			err = conn.SendJPEG(latest.DeviceImage)
		} else if profile.Transport == "serial-reva" && latest.OutputFormat == "png" {
			var rgb []byte
			rgb, err = rgb565(latest.DeviceImage)
			if err == nil && !latest.presenceCurrent() {
				latest = nil
				continue
			}
			if err == nil {
				err = conn.SendRGB565(rgb, request.Width, request.Height, request.Reverse180, config.Presentation.BlankOnUpdate)
			}
		} else {
			err = fmt.Errorf("unsupported TURZX image format")
		}
		if err != nil {
			conn.Close()
			conn = nil
			fail("turzx_send_failed", err)
			latest = nil
			retryAt = time.Now().Add(2 * time.Second)
			continue
		}
		lastFailure = ""
		shift := [2]int{request.ShiftX, request.ShiftY}
		if sentGeneration != request.Generation || sentPresence != latest.presence || sentShift != shift {
			o.logger.Info("turzx_frame_sent", "model", profile.Model, "width", request.Width, "height", request.Height, "generation", request.Generation, "away", latest.away, "shiftX", request.ShiftX, "shiftY", request.ShiftY)
		}
		sentGeneration = request.Generation
		sentPresence = latest.presence
		sentShift = shift
		latest = nil
	}
}

func rgb565(encoded []byte) ([]byte, error) {
	src, err := png.Decode(bytes.NewReader(encoded))
	if err != nil {
		return nil, err
	}
	bounds := src.Bounds()
	data := make([]byte, bounds.Dx()*bounds.Dy()*2)
	for y := bounds.Min.Y; y < bounds.Max.Y; y++ {
		for x := bounds.Min.X; x < bounds.Max.X; x++ {
			r, g, b, _ := src.At(x, y).RGBA()
			pixel := uint16((r>>11)<<11 | (g>>10)<<5 | b>>11)
			binary.LittleEndian.PutUint16(data[((y-bounds.Min.Y)*bounds.Dx()+x-bounds.Min.X)*2:], pixel)
		}
	}
	return data, nil
}

// Wait blocks until Run has returned.
func (o *Output) Wait() { <-o.done }

// rotateClockwise turns the 1920x462 image into the 462x1920 image the device expects.
func rotateClockwise(src *image.RGBA) *image.RGBA {
	b := src.Bounds()
	dst := image.NewRGBA(image.Rect(0, 0, b.Dy(), b.Dx()))
	for y := 0; y < b.Dy(); y++ {
		for x := 0; x < b.Dx(); x++ {
			s := src.PixOffset(b.Min.X+x, b.Min.Y+y)
			d := dst.PixOffset(b.Dy()-1-y, x)
			copy(dst.Pix[d:d+4], src.Pix[s:s+4])
		}
	}
	return dst
}
