//go:build windows

package settings

import (
	"bytes"
	"errors"
	"os"
	"reflect"
	"sync"
	"testing"

	"token-monitor-turzx/internal/display"
	"token-monitor-turzx/internal/fault"
	"token-monitor-turzx/internal/turzx"
)

func TestStyleCorrectionPreservesConnectionAndPreferencesAndCommitsOnce(t *testing.T) {
	for _, entry := range []string{"settings", "rendering"} {
		t.Run(entry, func(t *testing.T) {
			devices := []turzx.Device{}
			s, path := newService(t, &devices)
			ConfigureDisplay(s, presentationThemes())
			on := true
			prefs := &display.Presentation{Reverse180: true, BlankOnUpdate: true, SkipFull5h: true, Services: map[string]map[string]display.ServiceContent{
				"Local": {"Claude": {Enabled: false, Content: "Tokens"}}, "Hub": {"Codex": {Enabled: true, Content: "Limits"}},
			}}
			if _, err := s.Save(SaveRequest{Source: "Hub", URL: "https://hub.example.com", Token: "retained-secret", LimitStyle: "Bars", Cycle: &on, CycleSeconds: 37, ServiceCycleSeconds: 7, AwayMinutes: 23, Presentation: prefs}); err != nil {
				t.Fatal(err)
			}
			before, connection, err := s.read()
			if err != nil {
				t.Fatal(err)
			}
			before.HiddenLimits = []string{"retained-window"}
			if err := s.write(before); err != nil {
				t.Fatal(err)
			}
			generation, displaySignals, sourceSignals := s.generation, 0, 0
			s.OnDisplaySaved = func() { displaySignals++ }
			s.OnSaved = func() { sourceSignals++ }
			devices = []turzx.Device{{ID: smallDisplay}}
			if entry == "settings" {
				if _, err := s.Get(); err != nil {
					t.Fatal(err)
				}
			} else if config, err := DisplayConfiguration(s); err != nil || config.Theme.ID != "gauges-35-landscape" {
				t.Fatalf("configuration: %+v, %v", config, err)
			}
			after, afterConnection, err := s.read()
			before.LimitStyle = "gauges-35-landscape"
			if err != nil || !reflect.DeepEqual(after, before) || afterConnection != connection {
				t.Fatalf("correction changed other settings or the encrypted connection: %v", err)
			}
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			stat, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			for range 3 {
				if _, err := s.Get(); err != nil {
					t.Fatal(err)
				}
				if _, err := DisplayConfiguration(s); err != nil {
					t.Fatal(err)
				}
			}
			current, _ := os.ReadFile(path)
			currentStat, _ := os.Stat(path)
			if !bytes.Equal(data, current) || !stat.ModTime().Equal(currentStat.ModTime()) || s.generation != generation+1 || displaySignals != 1 || sourceSignals != 0 {
				t.Fatal("repeated configuration reads rewrote settings, advanced generation, or restarted the source")
			}
		})
	}
}

func TestConcurrentStyleCorrectionCommitsOnce(t *testing.T) {
	devices := []turzx.Device{{ID: smallDisplay}}
	s, _ := newService(t, &devices)
	ConfigureDisplay(s, presentationThemes())
	if err := s.write(file{Source: "Local", LimitStyle: "Bars"}); err != nil {
		t.Fatal(err)
	}
	signals := 0
	s.OnDisplaySaved = func() { signals++ }
	var group sync.WaitGroup
	errors := make(chan error, 16)
	for i := range 16 {
		group.Go(func() {
			if i%2 == 0 {
				_, err := s.Get()
				errors <- err
			} else {
				_, err := DisplayConfiguration(s)
				errors <- err
			}
		})
	}
	group.Wait()
	close(errors)
	for err := range errors {
		if err != nil {
			t.Fatal(err)
		}
	}
	saved, _, err := s.read()
	if err != nil || saved.LimitStyle != "gauges-35-landscape" || s.generation != 2 || signals != 1 {
		t.Fatalf("concurrent correction did not commit once: generation=%d signals=%d err=%v", s.generation, signals, err)
	}
}

func TestStyleCorrectionWriteFailurePreservesSelectionAndCanRecover(t *testing.T) {
	for _, entry := range []string{"settings", "rendering", "output save"} {
		t.Run(entry, func(t *testing.T) {
			devices := []turzx.Device{{ID: first}, {ID: smallDisplay}}
			s, path := newService(t, &devices)
			ConfigureDisplay(s, presentationThemes())
			before, err := s.Save(SaveRequest{Source: "Local", DisplayID: first, LimitStyle: "Bars", ServiceCycleSeconds: 7})
			if err != nil {
				t.Fatal(err)
			}
			if entry != "output save" {
				saved, _, _ := s.read()
				saved.DisplayID = ""
				saved.DisplayName = ""
				if err := s.write(saved); err != nil {
					t.Fatal(err)
				}
				devices = []turzx.Device{{ID: smallDisplay}}
			}
			data, _ := os.ReadFile(path)
			signals := 0
			s.OnDisplaySaved = func() { signals++ }
			if err := os.Chmod(path, 0o444); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
			apply := func() error {
				if entry == "output save" {
					_, err := s.Save(SaveRequest{Source: "Local", DisplayID: smallDisplay})
					return err
				}
				if entry == "settings" {
					_, err := s.Get()
					return err
				}
				_, err := DisplayConfiguration(s)
				return err
			}
			if apply() == nil {
				t.Fatal("read-only configuration was replaced")
			}
			current, _ := os.ReadFile(path)
			if !bytes.Equal(data, current) || s.generation != before.Generation || signals != 0 {
				t.Fatal("failed correction changed saved state or notified rendering")
			}
			if err := os.Chmod(path, 0o600); err != nil {
				t.Fatal(err)
			}
			if err := apply(); err != nil {
				t.Fatal(err)
			}
			after, err := s.Get()
			if err != nil || after.LimitStyle != "gauges-35-landscape" || after.Generation != before.Generation+1 || after.ServiceCycleSeconds != 7 || signals != 1 {
				t.Fatalf("correction did not recover with one commit: %+v, %v", after, err)
			}
			if entry == "output save" && after.DisplayID != smallDisplay {
				t.Fatal("output and corrected style were not saved together")
			}
		})
	}
}

func TestMissingDefaultStylePreservesIncompatibleSavedSelection(t *testing.T) {
	devices := []turzx.Device{{ID: smallDisplay}}
	s, path := newService(t, &devices)
	themes := presentationThemes()
	delete(themes, "gauges-35-landscape")
	ConfigureDisplay(s, themes)
	if err := s.write(file{Source: "Local", LimitStyle: "Bars"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	signals := 0
	s.OnDisplaySaved = func() { signals++ }
	_, err := s.Get()
	var public *fault.Error
	if !errors.As(err, &public) || public.Code != "STYLE_UNAVAILABLE" {
		t.Fatalf("missing default: %v", err)
	}
	if _, err := DisplayConfiguration(s); err == nil {
		t.Fatal("rendering accepted an unavailable default")
	}
	current, _ := os.ReadFile(path)
	if !bytes.Equal(data, current) || s.generation != 1 || signals != 0 {
		t.Fatal("missing default overwrote the old configuration")
	}
}
