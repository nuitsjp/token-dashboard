//go:build windows

package settings

import (
	"bytes"
	"encoding/json"
	"os"
	"strconv"
	"testing"

	"token-monitor-turzx/internal/turzx"
)

func TestServiceCycleDefaultsDoNotInheritLargeIntervalOrRewriteSettings(t *testing.T) {
	for _, seconds := range []int{0, 4, 3601, 5, 3600} {
		t.Run(strconv.Itoa(seconds), func(t *testing.T) {
			devices := []turzx.Device{}
			s, path := newService(t, &devices)
			ConfigureDisplay(s, presentationThemes())
			if err := s.write(file{Source: "Local", LimitStyle: "landscape", Cycle: false, CycleSeconds: 37, ServiceCycleSeconds: seconds}); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(path)
			view, err := s.Get()
			if err != nil {
				t.Fatal(err)
			}
			want := seconds
			if seconds < 5 || seconds > 3600 {
				want = 10
			}
			config, err := DisplayConfiguration(s)
			if err != nil || !config.Cycle || config.CycleSeconds != want || view.ServiceCycleSeconds != want || view.Cycle || view.CycleSeconds != 37 {
				t.Fatalf("unexpected intervals: view=%+v config=%+v err=%v", view, config, err)
			}
			after, _ := os.ReadFile(path)
			if !bytes.Equal(before, after) {
				t.Fatal("reading defaults rewrote settings")
			}
		})
	}
}

func TestServiceCycleValidationAndReplacementFailureKeepSavedState(t *testing.T) {
	devices := []turzx.Device{}
	s, path := newService(t, &devices)
	ConfigureDisplay(s, presentationThemes())
	before, err := s.Save(SaveRequest{Source: "Local", LimitStyle: "landscape", CycleSeconds: 37, ServiceCycleSeconds: 7})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	signals := 0
	s.OnDisplaySaved = func() { signals++ }
	for _, value := range []int{-1, 4, 3601} {
		_, err := s.Save(SaveRequest{Source: "Local", ServiceCycleSeconds: value})
		if fieldErrors(t, err)["serviceCycleSeconds"] == "" {
			t.Fatalf("invalid interval %d was accepted", value)
		}
	}
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	if _, err := s.Save(SaveRequest{Source: "Local", ServiceCycleSeconds: 9}); err == nil {
		t.Fatal("read-only settings were replaced")
	}
	after, err := s.Get()
	if err != nil {
		t.Fatal(err)
	}
	current, _ := os.ReadFile(path)
	if !bytes.Equal(data, current) || after.Generation != before.Generation || after.ServiceCycleSeconds != 7 || after.CycleSeconds != 37 || signals != 0 {
		t.Fatal("rejected save changed settings, generation, or rendering")
	}
}

func TestServiceCyclePersistsAcrossSourceAndStyleAndKeepsLargeSettings(t *testing.T) {
	devices := []turzx.Device{}
	s, path := newService(t, &devices)
	ConfigureDisplay(s, presentationThemes())
	on := true
	if _, err := s.Save(SaveRequest{Source: "Local", LimitStyle: "landscape", Cycle: &on, CycleSeconds: 37, ServiceCycleSeconds: 7}); err != nil {
		t.Fatal(err)
	}
	for _, value := range []int{5, 3600, 7} {
		if _, err := s.Save(SaveRequest{Source: "Local", ServiceCycleSeconds: value}); err != nil {
			t.Fatal(err)
		}
	}
	off := false
	if _, err := s.Save(SaveRequest{Source: "Local", Cycle: &off, CycleSeconds: 99}); err != nil {
		t.Fatal(err)
	}
	active, err := DisplayConfiguration(s)
	if err != nil || !active.Cycle || active.CycleSeconds != 7 {
		t.Fatalf("large settings changed the active service interval: %+v, %v", active, err)
	}
	if _, err := s.Save(SaveRequest{Source: "Local", Cycle: &on, CycleSeconds: 37}); err != nil {
		t.Fatal(err)
	}
	// Both omitted and zero interval requests retain the value when other settings change.
	if _, err := s.Save(SaveRequest{Source: "Hub", URL: "https://hub.example.com", Token: "test-secret", LimitStyle: "portrait"}); err != nil {
		t.Fatal(err)
	}
	reloaded := New(path, "test.app", s.list, s.logger)
	ConfigureDisplay(reloaded, presentationThemes())
	for _, style := range []string{"portrait", "landscape", "gauges", "bars"} {
		view, err := reloaded.Save(SaveRequest{Source: "Local", LimitStyle: style, ServiceCycleSeconds: 0})
		if err != nil {
			t.Fatal(err)
		}
		config, err := DisplayConfiguration(reloaded)
		want := 37
		if style == "portrait" || style == "landscape" {
			want = 7
		}
		if err != nil || view.ServiceCycleSeconds != 7 || !view.Cycle || view.CycleSeconds != 37 || !config.Cycle || config.CycleSeconds != want {
			t.Fatalf("style/source change lost independent intervals: view=%+v config=%+v err=%v", view, config, err)
		}
	}
	data, _ := os.ReadFile(path)
	var saved map[string]any
	if err := json.Unmarshal(data, &saved); err != nil {
		t.Fatal(err)
	}
	if saved["serviceCycleSeconds"] != float64(7) || saved["cycleSeconds"] != float64(37) || saved["cycle"] != true {
		t.Fatalf("intervals not persisted independently: %v", saved)
	}
}
