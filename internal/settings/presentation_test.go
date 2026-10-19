//go:build windows

package settings

import (
	"bytes"
	"os"
	"reflect"
	"testing"

	"token-monitor-turzx/internal/display"
	"token-monitor-turzx/internal/turzx"
)

const smallDisplay = `USB\VID_1A86&PID_5722\USB35INCHIPSV2`

func presentationThemes() map[string]display.Theme {
	return map[string]display.Theme{
		"gauges":              {ID: "gauges", Width: 1920, Height: 462, DataLayout: "gauges"},
		"bars":                {ID: "bars", Width: 1920, Height: 462, DataLayout: "bars"},
		"landscape":           {ID: "landscape", Width: 480, Height: 320, DataLayout: "service-gauges"},
		"portrait":            {ID: "portrait", Width: 320, Height: 480, DataLayout: "service-bars"},
		"gauges-35-landscape": {ID: "gauges-35-landscape", Width: 480, Height: 320, DataLayout: "service-gauges"},
	}
}

func TestCompatibleStylesFollowCapabilitiesAndCorrectIncompatibleSelection(t *testing.T) {
	for _, tc := range []struct {
		name      string
		target    string
		devices   []turzx.Device
		count     int
		warning   bool
		corrected bool
	}{
		{name: "Automatic offline", count: 5},
		{name: "Automatic small", devices: []turzx.Device{{ID: smallDisplay}}, count: 3, corrected: true},
		{name: "Automatic large", devices: []turzx.Device{{ID: first}}, count: 2},
		{name: "Automatic identifier order", devices: []turzx.Device{{ID: first}, {ID: smallDisplay}}, count: 3, corrected: true},
		{name: "explicit small offline", target: smallDisplay, count: 3, corrected: true},
		{name: "explicit large offline", target: first, count: 2},
		{name: "unknown saved device", target: "unknown", count: 0, warning: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			devices := tc.devices
			s, path := newService(t, &devices)
			ConfigureDisplay(s, presentationThemes())
			if err := s.write(file{Source: "Local", DisplayID: tc.target, LimitStyle: "Gauges"}); err != nil {
				t.Fatal(err)
			}
			before, _ := os.ReadFile(path)
			view, err := s.Get()
			if err != nil {
				t.Fatal(err)
			}
			style, generation := "gauges", uint64(1)
			if tc.corrected {
				style, generation = "gauges-35-landscape", 2
			}
			if len(view.CompatibleThemeIDs) != tc.count || (view.OutputWarning != "") != tc.warning || view.LimitStyle != style || len(view.Themes) != 5 {
				t.Fatalf("unexpected derived selection: %+v", view)
			}
			after, _ := os.ReadFile(path)
			if bytes.Equal(before, after) == tc.corrected || view.Generation != generation {
				t.Fatal("selection correction did not match saved state and generation")
			}
			if tc.name == "Automatic small" {
				devices = []turzx.Device{{ID: first}}
				view, _ = s.Get()
				if view.OutputWarning != "" || view.Generation != 3 || view.LimitStyle != "gauges" {
					t.Fatal("connection update did not correct the incompatible style")
				}
			}
		})
	}
}

func TestStyleSelectionRevalidatesTheCurrentAutomaticDevice(t *testing.T) {
	devices := []turzx.Device{{ID: smallDisplay}, {ID: first}}
	s, path := newService(t, &devices)
	ConfigureDisplay(s, presentationThemes())
	before, err := s.Save(SaveRequest{Source: "Local", LimitStyle: "landscape"})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	// The selector was populated for the small device, which disconnects before Save.
	devices = []turzx.Device{{ID: first}}
	_, err = s.Save(SaveRequest{Source: "Local", LimitStyle: "portrait"})
	if err != nil {
		t.Fatal(err)
	}
	after, _ := s.Get()
	current, _ := os.ReadFile(path)
	if after.Generation != before.Generation+1 || after.LimitStyle != "gauges" || bytes.Equal(data, current) {
		t.Fatal("stale style selection was not corrected for the current output")
	}
	// An output-only save keeps the corrected compatible style.
	after, err = s.Save(SaveRequest{Source: "Local", DisplayID: first})
	if err != nil || after.OutputWarning != "" || after.LimitStyle != "gauges" {
		t.Fatalf("output-only change: %+v, %v", after, err)
	}
}

func TestPresentationSaveFailurePreservesGenerationAndPreferences(t *testing.T) {
	devices := []turzx.Device{}
	s, path := newService(t, &devices)
	ConfigureDisplay(s, presentationThemes())
	prefs := &display.Presentation{SkipFull5h: true, Services: map[string]map[string]display.ServiceContent{"Local": {"Claude": {Enabled: false, Content: "Tokens"}}, "Hub": {"claude": {Enabled: true, Content: "Limits"}}}}
	before, err := s.Save(SaveRequest{Source: "Local", LimitStyle: "portrait", Presentation: prefs})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(path)
	signals := 0
	s.OnDisplaySaved = func() { signals++ }
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	next := display.ClonePresentation(prefs)
	next.Reverse180 = true
	if _, err := s.Save(SaveRequest{Source: "Local", Presentation: next}); err == nil {
		t.Fatal("read-only settings were replaced")
	}
	after, err := s.Get()
	if err != nil {
		t.Fatal(err)
	}
	current, _ := os.ReadFile(path)
	if !bytes.Equal(data, current) || after.Generation != before.Generation || !reflect.DeepEqual(after.Presentation, before.Presentation) || signals != 0 {
		t.Fatal("failed save changed visible state or notified rendering")
	}
}

func TestPresentationKeepsSourceSpecificMissingEntriesAndLegacyStyle(t *testing.T) {
	devices := []turzx.Device{}
	s, path := newService(t, &devices)
	ConfigureDisplay(s, presentationThemes())
	if err := s.write(file{Source: "Local", LimitStyle: "Bars"}); err != nil {
		t.Fatal(err)
	}
	_, err := s.Save(SaveRequest{Source: "Local", Presentation: &display.Presentation{Services: map[string]map[string]display.ServiceContent{"Local": {"Claude": {Enabled: false, Content: "Tokens"}}, "Hub": {"Claude": {Enabled: true, Content: "Limits"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = s.Save(SaveRequest{Source: "Local", Presentation: &display.Presentation{Reverse180: true, Services: map[string]map[string]display.ServiceContent{"Local": {"Codex": {Enabled: true, Content: "Both"}}}}})
	if err != nil {
		t.Fatal(err)
	}
	reloaded := New(path, "test.app", func() ([]turzx.Device, error) { return devices, nil }, s.logger)
	ConfigureDisplay(reloaded, presentationThemes())
	view, err := reloaded.Get()
	if err != nil {
		t.Fatal(err)
	}
	saved, _, _ := reloaded.read()
	if saved.LimitStyle != "Bars" || view.LimitStyle != "bars" || view.Presentation.Services["Local"]["Claude"].Enabled || view.Presentation.Services["Local"]["Claude"].Content != "Tokens" || view.Presentation.Services["Hub"]["Claude"].Content != "Limits" || !view.Presentation.Reverse180 {
		t.Fatal("unrelated save lost old style or per-source preferences")
	}
}
