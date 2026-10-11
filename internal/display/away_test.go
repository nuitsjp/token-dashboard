package display

import (
	"bytes"
	"context"
	"encoding/base64"
	"image/jpeg"
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
