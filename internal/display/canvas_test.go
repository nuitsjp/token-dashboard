package display

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"image"
	"image/jpeg"
	"image/png"
	"io"
	"log/slog"
	"testing"
	"time"

	"token-monitor-turzx/internal/usage"
)

func canvasService() *Service {
	return &Service{Logger: slog.New(slog.NewTextHandler(io.Discard, nil))}
}

func canvasFrame(t *testing.T) Frame {
	t.Helper()
	var pngBytes, jpegBytes bytes.Buffer
	if err := png.Encode(&pngBytes, image.NewRGBA(image.Rect(0, 0, Width, Height))); err != nil {
		t.Fatal(err)
	}
	if err := jpeg.Encode(&jpegBytes, image.NewRGBA(image.Rect(0, 0, Height, Width)), &jpeg.Options{Quality: 85}); err != nil {
		t.Fatal(err)
	}
	return Frame{PNG: pngBytes.Bytes(), JPEG: jpegBytes.Bytes()}
}

func TestCanvasFramePublishesOnlyTheMatchingRequest(t *testing.T) {
	s := canvasService()
	requested := make(chan struct{}, 1)
	r := NewCanvasRenderer(s, func(string, any) { requested <- struct{}{} })
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	done := make(chan frameResult, 1)
	go func() {
		frame, err := r.Render(ctx, nil, time.Now(), "Local", Bars)
		done <- frameResult{frame, err}
	}()
	<-requested
	request := s.RenderRequest()
	if request == nil || request.Theme != "bars" || request.Data.WaitingMessage != "Waiting for local usage" {
		t.Fatalf("request: %+v", request)
	}
	if err := s.CompleteFrame(request.ID+1, "", "", "stale page"); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-done:
		t.Fatalf("stale result completed the active frame: %+v", result)
	default:
	}
	frame := canvasFrame(t)
	if err := s.CompleteFrame(request.ID, base64.StdEncoding.EncodeToString(frame.PNG), base64.StdEncoding.EncodeToString(frame.JPEG), ""); err != nil {
		t.Fatal(err)
	}
	result := <-done
	if result.err != nil || !bytes.Equal(result.frame.PNG, frame.PNG) || !bytes.Equal(result.frame.JPEG, frame.JPEG) {
		t.Fatalf("frame did not preserve the Canvas encodings: %v", result.err)
	}
	if s.RenderRequest() != nil {
		t.Fatal("completed request was retained")
	}
}

func TestCanvasRenderCancellationAndFailure(t *testing.T) {
	for _, failure := range []bool{false, true} {
		s := canvasService()
		requested := make(chan struct{}, 1)
		r := NewCanvasRenderer(s, func(string, any) { requested <- struct{}{} })
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		done := make(chan error, 1)
		go func() {
			_, err := r.Render(ctx, nil, time.Now(), "Hub", Gauges)
			done <- err
		}()
		<-requested
		if failure {
			if err := s.CompleteFrame(s.RenderRequest().ID, "", "", "Image.decode failed"); err != nil {
				t.Fatal(err)
			}
		} else {
			cancel()
		}
		err := <-done
		if err == nil {
			t.Fatalf("failure=%v did not end rendering with an error", failure)
		}
		if !failure && !errors.Is(err, context.Canceled) {
			t.Fatalf("shutdown cancellation was not preserved: %v", err)
		}
		cancel()
		if s.RenderRequest() != nil {
			t.Fatal("failed request was retained")
		}
	}
}

func TestCanvasRenderTimeoutAllowsNextFrame(t *testing.T) {
	s := canvasService()
	requested := make(chan struct{}, 1)
	r := NewCanvasRenderer(s, func(string, any) { requested <- struct{}{} })
	r.responseTimeout = 20 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	done := make(chan frameResult, 1)
	render := func() {
		frame, err := r.Render(ctx, nil, time.Now(), "Local", Gauges)
		done <- frameResult{frame, err}
	}
	go render()
	<-requested
	expired := s.RenderRequest()
	select {
	case result := <-done:
		if !errors.Is(result.err, context.DeadlineExceeded) || ctx.Err() != nil {
			t.Fatalf("missing response did not time out independently of shutdown: %v", result.err)
		}
	case <-time.After(time.Second):
		t.Fatal("missing response blocked rendering")
	}
	if s.RenderRequest() != nil {
		t.Fatal("expired request was retained")
	}

	frame := canvasFrame(t)
	r.responseTimeout = time.Second
	go render()
	<-requested
	request := s.RenderRequest()
	if request == nil || request.ID <= expired.ID {
		t.Fatalf("request after expiry: %+v", request)
	}
	if err := s.CompleteFrame(expired.ID, "", "", "late response"); err != nil {
		t.Fatal(err)
	}
	select {
	case result := <-done:
		t.Fatalf("expired response completed the new request: %+v", result)
	default:
	}
	if err := s.CompleteFrame(request.ID, base64.StdEncoding.EncodeToString(frame.PNG), base64.StdEncoding.EncodeToString(frame.JPEG), ""); err != nil {
		t.Fatal(err)
	}
	result := <-done
	if result.err != nil || !bytes.Equal(result.frame.PNG, frame.PNG) || !bytes.Equal(result.frame.JPEG, frame.JPEG) {
		t.Fatalf("next frame did not recover: %v", result.err)
	}
}

func TestDisplayRunRecoversAfterCanvasTimeout(t *testing.T) {
	frame := canvasFrame(t)
	for _, trigger := range []string{"state change", "ticker"} {
		t.Run(trigger, func(t *testing.T) {
			s := canvasService()
			s.Hidden = func() ([]string, error) { return nil, nil }
			s.preview = "previous image"
			state := usage.NewState()
			ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
			requested := make(chan struct{}, 1)
			r := NewCanvasRenderer(s, func(string, any) {
				select {
				case requested <- struct{}{}:
				case <-ctx.Done():
				}
			})
			r.responseTimeout = 20 * time.Millisecond
			redraw := time.Hour
			if trigger == "ticker" {
				redraw = 50 * time.Millisecond
			}
			output := make(chan []byte, 1)
			updated := make(chan struct{}, 1)
			done := make(chan struct{})
			go func() {
				Run(ctx, s, r, nil, state, redraw, func() Style { return Gauges }, func(frame Frame) { output <- frame.JPEG }, func(string, any) { updated <- struct{}{} }, s.Logger)
				close(done)
			}()
			t.Cleanup(func() {
				cancel()
				<-done
			})
			<-requested
			if trigger == "state change" {
				state.Touch()
			}
			select {
			case <-requested:
			case <-time.After(time.Second):
				t.Fatal("display did not request another frame after a missing response")
			}
			if s.Preview() != "previous image" || len(output) != 0 || len(updated) != 0 {
				t.Fatal("timeout replaced the previous image")
			}
			request := s.RenderRequest()
			if err := s.CompleteFrame(request.ID, base64.StdEncoding.EncodeToString(frame.PNG), base64.StdEncoding.EncodeToString(frame.JPEG), ""); err != nil {
				t.Fatal(err)
			}
			select {
			case <-updated:
			case <-time.After(time.Second):
				t.Fatal("recovered frame did not update the preview")
			}
			if !bytes.Equal(<-output, frame.JPEG) || s.Preview() != "data:image/png;base64,"+base64.StdEncoding.EncodeToString(frame.PNG) {
				t.Fatal("recovered frame did not publish the matching PNG and JPEG")
			}
		})
	}
}

func TestCanvasRejectsInvalidImageDimensions(t *testing.T) {
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 1, 1))); err != nil {
		t.Fatal(err)
	}
	if _, err := decodeFrame(base64.StdEncoding.EncodeToString(encoded.Bytes()), "png", Width, Height); err == nil {
		t.Fatal("accepted an image that would disagree with the display dimensions")
	}
	if _, err := decodeFrame("invalid base64", "jpeg", Height, Width); err == nil {
		t.Fatal("accepted invalid image bytes")
	}
}

func TestOutputKeepsTheNewestEncodedJPEG(t *testing.T) {
	o := NewOutput(nil, nil, nil)
	o.Submit(Frame{JPEG: []byte("first")})
	o.Submit(Frame{JPEG: []byte("latest")})
	if got := string((<-o.pending).JPEG); got != "latest" || len(o.pending) != 0 {
		t.Fatalf("pending image: %q", got)
	}
}
