package display

import (
	"bytes"
	"context"
	"encoding/base64"
	"image"
	"image/png"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"token-monitor-turzx/internal/usage"
)

// The real scheduler and Canvas boundary run together; only the browser's encoded pixels are supplied.
func serviceRunner(t *testing.T, config Configuration, delay ...func() time.Duration) (*Service, chan FrameRequest, func(func(*Configuration))) {
	t.Helper()
	return serviceRunnerWithShift(t, config, 0, delay...)
}

func serviceRunnerWithShift(t *testing.T, config Configuration, shift time.Duration, delay ...func() time.Duration) (*Service, chan FrameRequest, func(func(*Configuration))) {
	t.Helper()
	s := canvasService()
	s.ShiftEvery = shift
	s.State = usage.NewState()
	s.State.SetSource(config.Source)
	s.Hidden = func() ([]string, error) { return nil, nil }
	var mu sync.Mutex
	s.Configuration = func() (Configuration, error) { mu.Lock(); defer mu.Unlock(); return config, nil }
	var encoded bytes.Buffer
	if err := png.Encode(&encoded, image.NewRGBA(image.Rect(0, 0, 480, 320))); err != nil {
		t.Fatal(err)
	}
	pixels := base64.StdEncoding.EncodeToString(encoded.Bytes())
	ctx, cancel := context.WithCancel(context.Background())
	var completions sync.WaitGroup
	renderer := NewCanvasRenderer(s, func(string, any) {
		r := s.RenderRequest()
		complete := func() {
			if err := s.CompleteFrame(r.ID, pixels, pixels, ""); err != nil {
				t.Errorf("Canvas completion: %v", err)
			}
		}
		if len(delay) > 0 {
			if wait := delay[0](); wait > 0 {
				completions.Add(1)
				go func() {
					defer completions.Done()
					timer := time.NewTimer(wait)
					defer timer.Stop()
					select {
					case <-ctx.Done():
					case <-timer.C:
						complete()
					}
				}()
				return
			}
		}
		complete()
	})
	frames := make(chan FrameRequest, 64)
	done := make(chan struct{})
	go func() {
		defer close(done)
		RunPresentation(ctx, s, renderer, time.Hour, func(frame Frame) {
			select {
			case frames <- frame.Request:
			case <-ctx.Done():
			}
		}, func(string, any) {})
	}()
	t.Cleanup(func() { cancel(); <-done; completions.Wait() })
	change := func(edit func(*Configuration)) { mu.Lock(); edit(&config); mu.Unlock(); s.State.Touch() }
	return s, frames, change
}

func nextServiceFrame(t *testing.T, frames <-chan FrameRequest, matches func(FrameRequest) bool) FrameRequest {
	t.Helper()
	deadline := time.NewTimer(2 * time.Second)
	defer deadline.Stop()
	for {
		select {
		case frame := <-frames:
			if matches(frame) {
				return frame
			}
		case <-deadline.C:
			t.Fatal("scheduler did not produce the expected service page")
			return FrameRequest{}
		}
	}
}

func scheduledServices(names ...string) *usage.Stats {
	stats := &usage.Stats{}
	for _, name := range names {
		stats.Limits.Providers = append(stats.Limits.Providers, usage.Provider{Provider: name, Windows: []usage.Window{quota("session", "5h", number(50))}})
	}
	return stats
}

func TestServiceUpdatesDoNotPostponeTheCycleDeadline(t *testing.T) {
	config := serviceConfig()
	config.Cycle, config.CycleEvery = true, 180*time.Millisecond
	s, frames, _ := serviceRunner(t, config)
	s.State.Set(scheduledServices("A", "B"))
	nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.Data.Service != nil && f.Data.Service.Provider == "A" })
	// More frequent data updates than the cycle interval must not starve page B.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	updated := make(chan struct{})
	go func() {
		defer close(updated)
		tick := time.NewTicker(20 * time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-tick.C:
				s.State.Set(scheduledServices("A", "B"))
			}
		}
	}()
	nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.Data.Service != nil && f.Data.Service.Provider == "B" })
	cancel()
	<-updated
}

func TestServiceAwayRetainsPageAndRestartsTheFullInterval(t *testing.T) {
	for _, removed := range []bool{false, true} {
		name := "retained"
		if removed {
			name = "removed"
		}
		t.Run(name, func(t *testing.T) {
			config := serviceConfig()
			config.Cycle, config.CycleEvery = true, 250*time.Millisecond
			s, frames, _ := serviceRunner(t, config)
			s.State.Set(scheduledServices("A", "B", "C"))
			nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.Data.Service != nil && f.Data.Service.Provider == "B" })
			SetAway(s, true)
			nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.Data.Service == nil })
			updated := scheduledServices("C", "A", "B")
			want := "B"
			if removed {
				updated, want = scheduledServices("A", "C"), "C"
			}
			s.State.Set(updated)
			nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.Data.Service == nil })
			SetAway(s, false)
			first := nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.Data.Service != nil })
			if first.Data.Service.Provider != want {
				t.Fatalf("returned to %s, want %s", first.Data.Service.Provider, want)
			}
			s.State.Touch()
			redraw := nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.Data.Service != nil })
			if redraw.Data.Service.Provider != want {
				t.Fatal("a data update advanced the resumed page")
			}
			select {
			case frame := <-frames:
				t.Fatalf("resumed page advanced before a full interval: %+v", frame.Data.Service)
			case <-time.After(100 * time.Millisecond):
			}
			nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.Data.Service != nil && f.Data.Service.Provider != want })
		})
	}
}

func TestResumeIntervalStartsAfterSlowCanvasCompletes(t *testing.T) {
	config := serviceConfig()
	config.Cycle, config.CycleEvery = true, 200*time.Millisecond
	var slowNext atomic.Bool
	s, frames, _ := serviceRunner(t, config, func() time.Duration {
		if slowNext.Swap(false) {
			return 300 * time.Millisecond
		}
		return 0
	})
	s.State.Set(scheduledServices("A", "B"))
	nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.Data.Service != nil && f.Data.Service.Provider == "A" })
	SetAway(s, true)
	nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.Data.Service == nil })
	slowNext.Store(true)
	SetAway(s, false)
	first := nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.Data.Service != nil })
	if first.Data.Service.Provider != "A" {
		t.Fatal("slow Canvas changed the resumed page")
	}
	select {
	case frame := <-frames:
		t.Fatalf("Canvas time consumed the resumed interval: %+v", frame.Data.Service)
	case <-time.After(100 * time.Millisecond):
	}
	nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.Data.Service != nil && f.Data.Service.Provider == "B" })
}

func TestServiceCursorFollowsIdentityAndTheNextSurvivingPage(t *testing.T) {
	config := serviceConfig()
	config.Cycle, config.CycleEvery = true, 80*time.Millisecond
	s, frames, change := serviceRunner(t, config)
	s.State.Set(scheduledServices("A", "B", "C"))
	nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.Data.Service != nil && f.Data.Service.Provider == "B" })
	change(func(c *Configuration) { c.CycleEvery = time.Hour; c.Generation++ })
	current := nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.Generation == 1 })
	if current.Data.Service.Provider != "B" {
		t.Fatal("interval change reset the page")
	}
	s.State.Set(scheduledServices("C", "A", "B"))
	frame := nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.ID > current.ID })
	if frame.Data.Service.Provider != "B" {
		t.Fatal("report reorder changed the current page identity")
	}
	// In the old order C,A,B, removing B must advance to C.
	s.State.Set(scheduledServices("A", "C"))
	current = nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.ID > frame.ID })
	if current.Data.Service.Provider != "C" {
		t.Fatal("removed page did not advance to the old order's next survivor")
	}
	change(func(c *Configuration) {
		c.Presentation.Reverse180 = true
		c.TargetID = "another output"
		c.Generation++
	})
	frame = nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.Generation == 2 })
	if frame.Data.Service.Provider != "C" || !frame.Reverse180 {
		t.Fatal("output-only changes reset the page")
	}
	change(func(c *Configuration) { c.Cycle = false; c.Generation++ })
	frame = nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.Generation == 3 })
	if frame.Data.Service.Provider != "A" {
		t.Fatal("Cycle OFF did not return to the first page")
	}
}

func TestSmallCanvasRejectsAnOldGenerationOrTarget(t *testing.T) {
	for _, changed := range []string{"generation", "target"} {
		t.Run(changed, func(t *testing.T) {
			s := canvasService()
			config := serviceConfig()
			config.Generation, config.TargetID = 4, "old output"
			s.Configuration = func() (Configuration, error) { return config, nil }
			requested := make(chan struct{}, 1)
			r := NewCanvasRenderer(s, func(string, any) { requested <- struct{}{} })
			done := make(chan error, 1)
			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			go func() {
				_, err := r.RenderPrepared(ctx, FrameRequest{Theme: "small", Width: 480, Height: 320, OutputFormat: "png", Generation: 4, TargetID: "old output"}, 1)
				done <- err
			}()
			<-requested
			if changed == "generation" {
				config.Generation++
			} else {
				config.TargetID = "new output"
			}
			// Configuration is checked before accepting any bytes from the stale response.
			if err := s.CompleteFrame(s.RenderRequest().ID, "", "", ""); err != nil {
				t.Fatal(err)
			}
			if err := <-done; err == nil {
				t.Fatal("an old output image was accepted")
			}
		})
	}
}
