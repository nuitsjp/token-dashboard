package display

import (
	"context"
	"image"
	"log/slog"
	"time"

	"token-monitor-turzx/internal/turzx"
)

// Frame keeps the render result and its destination together.
// Compact frames carry Image and PNG; Canvas frames carry PNG and encoded JPEG.
type Frame struct {
	Image  *image.RGBA
	PNG    []byte
	JPEG   []byte
	Target Options
}

// Output sends sequentially, retaining only the newest pending frame.
type Output struct {
	target  func() (Options, error)
	refresh func()
	logger  *slog.Logger
	pending chan Frame
	done    chan struct{}
}

func NewOutput(target func() (Options, error), refresh func(), logger *slog.Logger) *Output {
	return &Output{target: target, refresh: refresh, logger: logger, pending: make(chan Frame, 1), done: make(chan struct{})}
}

// Submit is called by the single drawing goroutine.
func (o *Output) Submit(frame Frame) {
	select {
	case <-o.pending:
	default:
	}
	o.pending <- frame
}

func (o *Output) Run(ctx context.Context) {
	defer close(o.done)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	var conn *turzx.Conn
	var connID, lastFailure string
	var sentOrientation string
	closeConn := func() {
		if conn != nil {
			conn.Close()
			conn = nil
		}
	}
	fail := func(event string, err error) {
		if err.Error() != lastFailure {
			lastFailure = err.Error()
			o.logger.Warn(event, "cause", err)
		}
	}
	for {
		var frame *Frame
		select {
		case <-ctx.Done():
			if conn != nil {
				if err := conn.Restart(); err != nil {
					o.logger.Warn("turzx_restart_failed", "cause", err)
				}
				closeConn()
			}
			return
		case latest := <-o.pending:
			frame = &latest
		case <-ticker.C:
		}
		options, err := o.target()
		if err != nil {
			fail("turzx_target_failed", err)
			continue
		}
		if conn != nil && (connID != options.DeviceID || !options.DeviceConnected) {
			closeConn()
		}
		if options.DeviceID == "" || !options.DeviceConnected {
			continue
		}
		if conn == nil {
			conn, err = turzx.Open(options.DeviceID)
			if err != nil {
				fail("turzx_open_failed", err)
				continue
			}
			connID = options.DeviceID
			sentOrientation = ""
			o.logger.Info("turzx_connected", "compact", options.Compact)
			// Ask for the page current now, rather than replaying an old pending page.
			o.refresh()
			continue
		}
		if frame == nil {
			continue
		}
		if frame.Target != options {
			o.refresh()
			continue
		}
		data := frame.JPEG
		if options.Compact {
			data = turzx.RGB565(frame.Image)
		}
		current, err := o.target()
		if err != nil {
			fail("turzx_target_failed", err)
			continue
		}
		if current != options {
			o.refresh()
			continue
		}
		if options.Compact {
			err = conn.SendRGB565(data, options.Orientation)
		} else {
			err = conn.SendJPEG(data)
		}
		if err != nil {
			closeConn()
			fail("turzx_send_failed", err)
			continue
		}
		lastFailure = ""
		if sentOrientation != options.Orientation {
			width, height := Width, Height
			if options.Compact {
				width, height = frame.Image.Bounds().Dx(), frame.Image.Bounds().Dy()
			}
			o.logger.Info("turzx_frame_sent", "compact", options.Compact, "orientation", options.Orientation, "width", width, "height", height)
			sentOrientation = options.Orientation
		}
	}
}

func (o *Output) Wait() { <-o.done }

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
