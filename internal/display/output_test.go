package display

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

type recordingOutputConnection struct {
	id         string
	events     chan string
	restartErr error
}

func (c *recordingOutputConnection) SendJPEG([]byte) error { c.events <- c.id + ":send"; return nil }
func (c *recordingOutputConnection) SendRGB565([]byte, int, int, bool, bool) error {
	c.events <- c.id + ":send"
	return nil
}
func (c *recordingOutputConnection) Restart() error {
	c.events <- c.id + ":restart"
	return c.restartErr
}
func (c *recordingOutputConnection) Close() error { c.events <- c.id + ":close"; return nil }

func TestOutputRestartsOnlyWhenTheDeviceChanges(t *testing.T) {
	for _, scenario := range []string{"no replacement image", "old generation", "stale presence", "new image", "restart fails", "same device", "disconnected"} {
		t.Run(scenario, func(t *testing.T) {
			const oldID = `USB\VID_1CBE&PID_0092\OLD`
			const newID = `USB\VID_1CBE&PID_0092\NEW`
			config := Configuration{TargetID: oldID, TargetConnected: true, Generation: 1, Theme: Theme{ID: "large", Width: Width, Height: Height}}
			var mu sync.Mutex
			events := make(chan string, 20)
			ticks := make(chan time.Time)
			o := NewOutput(func() (Configuration, error) { mu.Lock(); defer mu.Unlock(); return config, nil }, slog.New(slog.NewTextHandler(io.Discard, nil)))
			o.pending = make(chan Frame)
			ctx, cancel := context.WithCancel(context.Background())
			defer func() { cancel(); o.Wait() }()
			go o.run(ctx, ticks, func(id string) (outputConnection, error) {
				events <- id + ":open"
				c := &recordingOutputConnection{id: id, events: events}
				if scenario == "restart fails" && id == oldID {
					c.restartErr = errors.New("restart failed")
				}
				return c, nil
			})
			expect := func(want string) {
				t.Helper()
				select {
				case got := <-events:
					if got != want {
						t.Fatalf("event = %q, want %q", got, want)
					}
				case <-time.After(3 * time.Second):
					t.Fatalf("missing event %q", want)
				}
			}
			frame := Frame{Request: presentationRequest(config, 0), OutputFormat: "jpeg"}
			o.Submit(frame)
			expect(oldID + ":open")
			expect(oldID + ":send")
			mu.Lock()
			config.Generation++
			if scenario != "same device" && scenario != "disconnected" {
				config.TargetID = newID
			}
			if scenario == "disconnected" {
				config.TargetConnected = false
			}
			next := Frame{Request: presentationRequest(config, 0), OutputFormat: "jpeg"}
			mu.Unlock()
			switch scenario {
			case "no replacement image", "disconnected":
				ticks <- time.Now()
			case "old generation", "same device":
				// The old image must be discarded without losing the old device connection.
				o.Submit(frame)
				// The next tick is received only after that image has been processed.
				ticks <- time.Now()
			case "stale presence":
				presence := make(chan struct{})
				close(presence)
				frame.presence = presence
				o.Submit(frame)
			default:
				o.Submit(next)
			}
			if scenario == "same device" {
				o.Submit(next)
				expect(oldID + ":send")
				// A later device change must still restart the retained connection.
				mu.Lock()
				config.TargetID = newID
				config.Generation++
				next.Request = presentationRequest(config, 0)
				mu.Unlock()
				ticks <- time.Now()
			}
			if scenario != "disconnected" {
				expect(oldID + ":restart")
			}
			expect(oldID + ":close")
			if scenario == "disconnected" {
				return
			}
			if scenario != "new image" && scenario != "restart fails" {
				o.Submit(next)
			}
			expect(newID + ":open")
			expect(newID + ":send")
		})
	}
}
