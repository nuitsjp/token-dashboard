package display

import (
	"context"
	"testing"
	"time"
)

var expectedShifts = [][2]int{{0, 0}, {4, 0}, {4, 4}, {0, 4}, {-4, 4}, {-4, 0}, {-4, -4}, {0, -4}}

func TestPresentationShiftDimensionsAndTransforms(t *testing.T) {
	if got := (&Service{}).shiftEvery(); got != 3*time.Minute {
		t.Fatalf("default shift interval: %v", got)
	}
	for _, size := range [][2]int{{480, 320}, {320, 480}, {Width, Height}} {
		for _, reverse := range []bool{false, true} {
			config := Configuration{Theme: Theme{ID: "selected", Width: size[0], Height: size[1]}, TargetID: "device", Generation: 7, Presentation: Presentation{Reverse180: reverse}}
			for step, offset := range expectedShifts {
				request := presentationRequest(config, step)
				format, flipped := "png", reverse
				if size[0] == Width {
					format, flipped = "jpeg", false
				}
				if request.Width != size[0] || request.Height != size[1] || request.OutputFormat != format || request.Reverse180 != flipped || request.ShiftX != offset[0] || request.ShiftY != offset[1] || request.TargetID != "device" || request.Generation != 7 {
					t.Fatalf("size=%v reverse=%v step=%d: %+v", size, reverse, step, request)
				}
			}
		}
	}
}

func TestSmallShiftKeepsItsDeadlineAcrossUpdatesAndConfiguration(t *testing.T) {
	config := serviceConfig()
	config.Cycle, config.CycleEvery = true, 120*time.Millisecond
	s, frames, change := serviceRunnerWithShift(t, config, 180*time.Millisecond)
	s.State.Set(scheduledServices("A", "B"))
	nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.Data.Service != nil })
	ctx, cancel := context.WithCancel(context.Background())
	updated := make(chan struct{})
	go func() {
		defer close(updated)
		tick := time.NewTicker(15 * time.Millisecond)
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
	defer func() { cancel(); <-updated }()
	providers := map[string]bool{}
	deadline := time.NewTimer(3 * time.Second)
	defer deadline.Stop()
	step := 0
	for step < len(expectedShifts) {
		select {
		case f := <-frames:
			if f.Data.Service != nil {
				providers[f.Data.Service.Provider] = true
			}
			offset := [2]int{f.ShiftX, f.ShiftY}
			if offset == expectedShifts[step%len(expectedShifts)] {
				continue
			}
			step++
			if offset != expectedShifts[step%len(expectedShifts)] {
				t.Fatalf("shift %d: %v", step, offset)
			}
			if step == 3 {
				// Changing Style, flip and output must preserve both position and its deadline.
				change(func(c *Configuration) {
					c.Theme.ID, c.Theme.DataLayout = "other-style", "service-bars"
					c.Presentation.Reverse180 = true
					c.TargetID, c.Generation = "other-device", 9
				})
				changed := nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.Generation == 9 })
				if changed.ShiftX != offset[0] || changed.ShiftY != offset[1] || !changed.Reverse180 || changed.Theme != "other-style" || changed.TargetID != "other-device" {
					t.Fatalf("configuration reset the shift: %+v", changed)
				}
			}
		case <-deadline.C:
			t.Fatal("updates or page/configuration changes postponed the eight-point shift cycle")
		}
	}
	if len(providers) != 2 {
		t.Fatalf("Cycle did not advance independently: %v", providers)
	}
}

func TestSmallShiftStopsAwayAndRestartsWithAFullInterval(t *testing.T) {
	config := serviceConfig()
	const interval = 200 * time.Millisecond
	s, frames, _ := serviceRunnerWithShift(t, config, interval)
	s.State.Set(scheduledServices("A"))
	nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.ShiftX == 4 && f.ShiftY == 4 })
	SetAway(s, true)
	black := nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.Data.Service == nil })
	time.Sleep(2 * interval)
	s.State.Touch()
	still := nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.ID == 0 })
	if still.ShiftX != black.ShiftX || still.ShiftY != black.ShiftY {
		t.Fatal("shift advanced while away")
	}
	SetAway(s, false)
	resumed := nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.Data.Service != nil })
	if resumed.ShiftX != 0 || resumed.ShiftY != 0 {
		t.Fatalf("resumed shift: %d,%d", resumed.ShiftX, resumed.ShiftY)
	}
	s.State.Touch()
	redraw := nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.Data.Service != nil })
	if redraw.ShiftX != 0 || redraw.ShiftY != 0 {
		t.Fatal("update consumed the resumed interval")
	}
	select {
	case frame := <-frames:
		t.Fatalf("shift resumed before a full interval: %+v", frame)
	case <-time.After(interval / 2):
	}
	nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.ShiftX == 4 && f.ShiftY == 0 })
}

func TestSmallConfigurationDoesNotRestartTheShiftClock(t *testing.T) {
	s, frames, change := serviceRunnerWithShift(t, serviceConfig(), 650*time.Millisecond)
	s.State.Set(scheduledServices("A"))
	nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.ShiftX == 4 && f.ShiftY == 0 })
	time.Sleep(420 * time.Millisecond)
	change(func(c *Configuration) {
		c.Theme.ID, c.Theme.DataLayout = "bars", "service-bars"
		c.Presentation.Reverse180 = true
		c.TargetID, c.Generation = "new output", 1
	})
	changed := nextServiceFrame(t, frames, func(f FrameRequest) bool { return f.Generation == 1 })
	if changed.ShiftX != 4 || changed.ShiftY != 0 {
		t.Fatal("configuration changed the current offset")
	}
	// About 230ms remain on the existing clock; resetting it would require another 650ms.
	select {
	case next := <-frames:
		if next.ShiftX != 4 || next.ShiftY != 4 {
			t.Fatalf("next scheduled offset: %d,%d", next.ShiftX, next.ShiftY)
		}
	case <-time.After(400 * time.Millisecond):
		t.Fatal("configuration postponed the next shift")
	}
}
