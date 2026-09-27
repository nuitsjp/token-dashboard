package display

import (
	"bytes"
	"context"
	"image"
	"image/jpeg"
	"log/slog"

	"token-monitor-turzx/internal/turzx"
)

// Output sends images to one TURZX in order. Only the newest pending image is kept.
type Output struct {
	target  func() (string, error)
	logger  *slog.Logger
	pending chan *image.RGBA
	done    chan struct{}
}

// NewOutput sends to the device ID that target returns at the time of each send.
func NewOutput(target func() (string, error), logger *slog.Logger) *Output {
	return &Output{target: target, logger: logger, pending: make(chan *image.RGBA, 1), done: make(chan struct{})}
}

// Submit replaces any image still waiting to be sent. It must be called from one goroutine.
func (o *Output) Submit(img *image.RGBA) {
	select {
	case <-o.pending:
	default:
	}
	o.pending <- img
}

// Run sends until ctx ends, then restarts the display so it returns to its start-up screen.
func (o *Output) Run(ctx context.Context) {
	defer close(o.done)
	var conn *turzx.Conn
	var connID, lastFailure string
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
		case img := <-o.pending:
			id, err := o.target()
			if err != nil {
				fail("turzx_target_failed", err)
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
					continue
				}
				connID = id
			}
			var buf bytes.Buffer
			if err := jpeg.Encode(&buf, rotateClockwise(img), &jpeg.Options{Quality: 85}); err != nil {
				fail("turzx_encode_failed", err)
				continue
			}
			// A failed image is dropped; the next image reopens the device.
			if err := conn.SendJPEG(buf.Bytes()); err != nil {
				conn.Close()
				conn = nil
				fail("turzx_send_failed", err)
				continue
			}
			lastFailure = ""
		}
	}
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
