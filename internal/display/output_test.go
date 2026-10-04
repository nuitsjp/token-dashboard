package display

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/draw"
	"image/jpeg"
	"io"
	"log/slog"
	"sync/atomic"
	"testing"
	"time"
)

type outputEvent struct {
	operation string
	id        string
	jpeg      []byte
}

type fakeOutputConn struct {
	id      string
	events  chan outputEvent
	release <-chan struct{}
	sendErr error
}

func (c *fakeOutputConn) SendJPEG(data []byte) error {
	c.events <- outputEvent{operation: "send", id: c.id, jpeg: append([]byte(nil), data...)}
	if c.release != nil {
		<-c.release
	}
	return c.sendErr
}

func (c *fakeOutputConn) Restart() error {
	c.events <- outputEvent{operation: "restart", id: c.id}
	return nil
}

func (c *fakeOutputConn) Close() error {
	c.events <- outputEvent{operation: "close", id: c.id}
	return nil
}

func startTestOutput(t *testing.T, target func() (string, error), open func(string) (outputConn, error)) (*Output, func()) {
	t.Helper()
	o := NewOutput(target, slog.New(slog.NewTextHandler(io.Discard, nil)))
	o.open = open
	ctx, cancel := context.WithCancel(context.Background())
	go o.Run(ctx)
	stop := func() {
		cancel()
		select {
		case <-o.done:
		case <-time.After(3 * time.Second):
			t.Error("output did not stop")
		}
	}
	t.Cleanup(stop)
	return o, stop
}

func nextOutputEvent(t *testing.T, events <-chan outputEvent, operation, id string) outputEvent {
	t.Helper()
	select {
	case event := <-events:
		if event.operation != operation || event.id != id {
			t.Fatalf("event = %s %s, want %s %s", event.operation, event.id, operation, id)
		}
		return event
	case <-time.After(3 * time.Second):
		t.Fatalf("no %s event for %s", operation, id)
		return outputEvent{}
	}
}

func outputImage(level uint8) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, 8, 4))
	draw.Draw(img, img.Bounds(), image.NewUniform(color.Gray{Y: level}), image.Point{}, draw.Src)
	return img
}

func checkOutputImage(t *testing.T, event outputEvent, level uint8) {
	t.Helper()
	img, err := jpeg.Decode(bytes.NewReader(event.jpeg))
	if err != nil {
		t.Fatal(err)
	}
	if img.Bounds().Dx() != 4 || img.Bounds().Dy() != 8 {
		t.Fatalf("sent image size = %v, want 4x8", img.Bounds())
	}
	got := color.GrayModel.Convert(img.At(0, 0)).(color.Gray).Y
	if delta := int(got) - int(level); delta < -1 || delta > 1 {
		t.Fatalf("sent image level = %d, want %d", got, level)
	}
}

func TestOutputKeepsOnlyLatestPendingImage(t *testing.T) {
	events := make(chan outputEvent, 16)
	release := make(chan struct{})
	conn := &fakeOutputConn{id: "display", events: events, release: release}
	o, stop := startTestOutput(t, func() (string, error) { return "display", nil }, func(id string) (outputConn, error) {
		events <- outputEvent{operation: "open", id: id}
		return conn, nil
	})
	// Release a blocked send even when an assertion fails, before stopping the output.
	t.Cleanup(func() {
		select {
		case <-release:
		default:
			close(release)
		}
	})
	o.Submit(outputImage(20))
	nextOutputEvent(t, events, "open", "display")
	checkOutputImage(t, nextOutputEvent(t, events, "send", "display"), 20)
	o.Submit(outputImage(100))
	o.Submit(outputImage(220))
	close(release)
	checkOutputImage(t, nextOutputEvent(t, events, "send", "display"), 220)
	stop()
	nextOutputEvent(t, events, "restart", "display")
	nextOutputEvent(t, events, "close", "display")
	if len(events) != 0 {
		t.Fatalf("unexpected extra output events: %d", len(events))
	}
}

func TestOutputReopensAfterSendFailure(t *testing.T) {
	events := make(chan outputEvent, 16)
	first := &fakeOutputConn{id: "first", events: events, sendErr: errors.New("disconnected")}
	second := &fakeOutputConn{id: "second", events: events}
	connections := []outputConn{first, second}
	o, _ := startTestOutput(t, func() (string, error) { return "display", nil }, func(id string) (outputConn, error) {
		events <- outputEvent{operation: "open", id: id}
		if len(connections) == 0 {
			return nil, errors.New("unexpected extra open")
		}
		conn := connections[0]
		connections = connections[1:]
		return conn, nil
	})
	o.Submit(outputImage(20))
	nextOutputEvent(t, events, "open", "display")
	checkOutputImage(t, nextOutputEvent(t, events, "send", "first"), 20)
	nextOutputEvent(t, events, "close", "first")
	o.Submit(outputImage(220))
	nextOutputEvent(t, events, "open", "display")
	checkOutputImage(t, nextOutputEvent(t, events, "send", "second"), 220)
}

func TestOutputClosesPreviousTargetBeforeOpeningNext(t *testing.T) {
	events := make(chan outputEvent, 16)
	var target atomic.Value
	target.Store("first")
	o, _ := startTestOutput(t, func() (string, error) { return target.Load().(string), nil }, func(id string) (outputConn, error) {
		events <- outputEvent{operation: "open", id: id}
		return &fakeOutputConn{id: id, events: events}, nil
	})
	o.Submit(outputImage(20))
	nextOutputEvent(t, events, "open", "first")
	nextOutputEvent(t, events, "send", "first")
	target.Store("second")
	o.Submit(outputImage(220))
	nextOutputEvent(t, events, "close", "first")
	nextOutputEvent(t, events, "open", "second")
	checkOutputImage(t, nextOutputEvent(t, events, "send", "second"), 220)
}

func TestOutputRestartsAndClosesBeforeStopping(t *testing.T) {
	events := make(chan outputEvent, 16)
	o, stop := startTestOutput(t, func() (string, error) { return "display", nil }, func(id string) (outputConn, error) {
		return &fakeOutputConn{id: id, events: events}, nil
	})
	o.Submit(outputImage(20))
	nextOutputEvent(t, events, "send", "display")
	stop()
	nextOutputEvent(t, events, "restart", "display")
	nextOutputEvent(t, events, "close", "display")
	if len(events) != 0 {
		t.Fatalf("unexpected extra output events: %d", len(events))
	}
}
