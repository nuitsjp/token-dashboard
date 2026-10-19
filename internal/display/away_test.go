package display

import (
	"bytes"
	"context"
	"encoding/base64"
	"image/jpeg"
	"image/png"
	"testing"
	"time"

	"token-monitor-turzx/internal/usage"
)

func TestBlackImageFitsTheDisplay(t *testing.T) {
	config, err := jpeg.DecodeConfig(bytes.NewReader(black()))
	if err != nil || config.Width != Height || config.Height != Width {
		t.Fatalf("black image: %+v, %v", config, err)
	}
}

func TestBlackFramesMatchEveryStyleAndRotation(t *testing.T) {
	for _, size := range [][2]int{{480, 320}, {320, 480}, {Width, Height}} {
		for _, reverse := range []bool{false, true} {
			config := Configuration{Theme: Theme{Width: size[0], Height: size[1]}, Presentation: Presentation{Reverse180: reverse}}
			frame, err := blackFrame(presentationRequest(config, 0))
			if err != nil {
				t.Fatal(err)
			}
			if !frame.away || len(frame.PNG) != 0 {
				t.Fatal("black output must not replace the preview")
			}
			if size[0] == Width {
				image, err := jpeg.DecodeConfig(bytes.NewReader(frame.DeviceImage))
				if err != nil || image.Width != Height || image.Height != Width || frame.OutputFormat != "jpeg" {
					t.Fatalf("large black frame: %+v, %v", image, err)
				}
				continue
			}
			image, err := png.DecodeConfig(bytes.NewReader(frame.DeviceImage))
			if err != nil || image.Width != size[0] || image.Height != size[1] || frame.Request.Reverse180 != reverse || frame.OutputFormat != "png" {
				t.Fatalf("small black frame: %+v, %v", image, err)
			}
			pixels, err := rgb565(frame.DeviceImage)
			if err != nil || len(pixels) != size[0]*size[1]*2 || !bytes.Equal(pixels, make([]byte, len(pixels))) {
				t.Fatalf("black serial pixels: %d bytes, %v", len(pixels), err)
			}
		}
	}
}

func TestAwayCancelsCanvasAndInvalidatesQueuedFrames(t *testing.T) {
	s := canvasService()
	requested := make(chan FrameRequest, 1)
	renderer := NewCanvasRenderer(s, func(string, any) { requested <- *s.RenderRequest() })
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, err := renderer.RenderPrepared(ctx, FrameRequest{Width: 480, Height: 320, OutputFormat: "png"}, 1)
		done <- err
	}()
	request := <-requested
	_, present := s.awayState()
	queued := Frame{presence: present}
	SetAway(s, true)
	select {
	case err := <-done:
		if err != errPresenceChanged {
			t.Fatalf("pending Canvas was not cancelled: %v", err)
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("away waited for the Canvas response timeout")
	}
	if queued.presenceCurrent() {
		t.Fatal("a queued normal image survived the away transition")
	}
	// An old Canvas reply must not become the current image, even after an immediate return.
	SetAway(s, false)
	if err := s.CompleteFrame(request.ID, "invalid old pixels", "invalid old pixels", ""); err != nil {
		t.Fatalf("old reply was decoded: %v", err)
	}
	if s.Preview() != "" || queued.presenceCurrent() {
		t.Fatal("old frame became current after return")
	}
	_, current := s.awayState()
	SetAway(s, false)
	if !(Frame{presence: current}).presenceCurrent() {
		t.Fatal("a repeated presence notification invalidated the current frame")
	}
	SetAway(s, true)
	_, away := s.awayState()
	black := Frame{presence: away, away: true}
	SetAway(s, false)
	if black.presenceCurrent() {
		t.Fatal("a queued black image survived return")
	}
}

// While away only black goes to the display, and back the latest image does.
func TestRunSendsBlackWhileAway(t *testing.T) {
	frame := canvasFrame(t)
	s := canvasService()
	s.Hidden = func() ([]string, error) { return nil, nil }
	state := usage.NewState()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	r := NewCanvasRenderer(s, func(name string, _ any) {
		if name != RenderRequested {
			return
		}
		go func() {
			if req := s.RenderRequest(); req != nil {
				_ = s.CompleteFrame(req.ID, base64.StdEncoding.EncodeToString(frame.PNG), base64.StdEncoding.EncodeToString(frame.JPEG), "")
			}
		}()
	})
	out := make(chan []byte, 64)
	go Run(ctx, s, r, state, 100*time.Millisecond, func() Style { return Gauges }, func(j []byte) { out <- j }, func(string, any) {}, s.Logger)
	next := func() []byte {
		select {
		case j := <-out:
			return j
		case <-time.After(2 * time.Second):
			t.Fatal("nothing was sent")
			return nil
		}
	}
	if !bytes.Equal(next(), frame.JPEG) {
		t.Fatal("the first image was not the drawn one")
	}
	SetAway(s, true)
	for !bytes.Equal(next(), black()) {
	}
	state.Touch()
	for i := 0; i < 5; i++ {
		if !bytes.Equal(next(), black()) {
			t.Fatal("an image other than black was sent while away")
		}
	}
	SetAway(s, false)
	for i := 0; i < 20; i++ {
		if bytes.Equal(next(), frame.JPEG) {
			return
		}
	}
	t.Fatal("the display did not come back")
}
