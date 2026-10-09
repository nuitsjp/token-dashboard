//go:build windows

package settings

import (
	"bytes"
	"encoding/json"
	"os"
	"reflect"
	"testing"

	"token-monitor-turzx/internal/turzx"
)

func TestCompactServiceChoicesPersistWithoutLosingOtherSettings(t *testing.T) {
	devices := []turzx.Device{}
	s, path := newService(t, &devices)
	if err := os.WriteFile(path, []byte(`{"source":"Local","orientation":"ReversePortrait","rotationIntervalSeconds":30,"skipFull5hServices":true,"hiddenLimits":["hidden"],"compactServiceContent":{"temporary":{"showLimits":false},"Claude":{"showTokens":false}}}`), 0600); err != nil {
		t.Fatal(err)
	}
	for _, choice := range []struct{ enabled, limits, tokens bool }{
		{true, true, true}, {true, true, false}, {true, false, true},
		{false, true, true}, {false, true, false}, {false, false, true},
	} {
		if err := SetCompactService(s, "claude", choice.enabled, choice.limits, choice.tokens); err != nil {
			t.Fatal(err)
		}
		if _, err := s.Save(SaveRequest{Source: "Local", LimitStyle: "Bars"}); err != nil {
			t.Fatal(err)
		}
		if err := SetLimitsShown(s, []string{"second-hidden"}, false); err != nil {
			t.Fatal(err)
		}
		reloaded := New(path, "test.app", func() ([]turzx.Device, error) { return devices, nil }, s.logger)
		content, err := CompactServices(reloaded)
		if err != nil || content["claude"].Enabled == nil || *content["claude"].Enabled != choice.enabled || content["claude"].ShowLimits == nil || content["claude"].ShowTokens == nil || *content["claude"].ShowLimits != choice.limits || *content["claude"].ShowTokens != choice.tokens {
			t.Fatalf("restored choice %+v, %v", content, err)
		}
		if content["temporary"].ShowTokens != nil || *content["temporary"].ShowLimits || content["Claude"].ShowLimits != nil || *content["Claude"].ShowTokens {
			t.Fatal("other/partially saved service changed")
		}
		view, err := reloaded.Get()
		hidden, _ := HiddenLimits(reloaded)
		if err != nil || view.Orientation != "ReversePortrait" || view.RotationIntervalSeconds != 30 || !view.SkipFull5hServices || !reflect.DeepEqual(hidden, []string{"hidden", "second-hidden"}) {
			t.Fatalf("other settings lost: %+v, %v", view, hidden)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		var saved map[string]json.RawMessage
		if err := json.Unmarshal(data, &saved); err != nil {
			t.Fatal(err)
		}
		if len(saved["compactServiceContent"]) == 0 {
			t.Fatal("no persisted choices")
		}
	}
}

func TestCompactServiceFailedWriteKeepsPreviousChoice(t *testing.T) {
	devices := []turzx.Device{}
	s, path := newService(t, &devices)
	if err := SetCompactService(s, "tool", false, true, false); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0444); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0600)
	if err := SetCompactService(s, "tool", true, false, true); err == nil {
		t.Fatal("read-only save succeeded")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("failed service save changed file")
	}
	content, err := CompactServices(s)
	if err != nil || *content["tool"].Enabled || !*content["tool"].ShowLimits || *content["tool"].ShowTokens {
		t.Fatalf("failed save changed choice: %+v", content)
	}
}

func TestCompactServiceRejectsEmptyContentWithoutChangingFile(t *testing.T) {
	devices := []turzx.Device{}
	s, path := newService(t, &devices)
	if err := SetCompactService(s, "tool", false, false, true); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, enabled := range []bool{false, true} {
		if err := SetCompactService(s, "tool", enabled, false, false); err == nil {
			t.Fatal("empty content choice was saved")
		}
		after, err := os.ReadFile(path)
		if err != nil || !bytes.Equal(before, after) {
			t.Fatal("invalid content changed settings")
		}
	}
}

func TestCompactDefaultsForMissingAndLegacySettings(t *testing.T) {
	devices := []turzx.Device{}
	s, path := newService(t, &devices)
	for _, legacy := range []bool{false, true} {
		if legacy {
			if err := os.WriteFile(path, []byte(`{"source":"Local","displayID":"","displayName":""}`), 0600); err != nil {
				t.Fatal(err)
			}
		}
		view, err := s.Get()
		if err != nil || view.Orientation != "Landscape" || view.RotationIntervalSeconds != 10 || view.SkipFull5hServices {
			t.Fatalf("defaults: %+v, %v", view, err)
		}
	}
}

func TestCompactSettingsPersistAndOtherSavesKeepThem(t *testing.T) {
	const id = `USB\VID_1A86&PID_5722\USB35INCHIPSV2`
	devices := []turzx.Device{{ID: id, Name: "TURZX 3.5-inch (USB35INC)"}}
	s, path := newService(t, &devices)
	for _, orientation := range []string{"Landscape", "ReverseLandscape", "Portrait", "ReversePortrait"} {
		interval, skip := 300, true
		if _, err := s.Save(SaveRequest{Source: "Local", DisplayID: id, Orientation: &orientation, RotationIntervalSeconds: &interval, SkipFull5hServices: &skip}); err != nil {
			t.Fatal(err)
		}
		for _, request := range []SaveRequest{{Source: "Local", DisplayID: id, LimitStyle: "Bars"}, {Source: "Local", DisplayID: id}} {
			if _, err := s.Save(request); err != nil {
				t.Fatal(err)
			}
		}
		if err := SetLimitsShown(s, []string{"hidden"}, false); err != nil {
			t.Fatal(err)
		}
		devices = nil
		reloaded := New(path, "test.app", func() ([]turzx.Device, error) { return devices, nil }, s.logger)
		view, err := reloaded.Get()
		if err != nil || view.Orientation != orientation || view.RotationIntervalSeconds != 300 || !view.SkipFull5hServices || view.DisplayID != id || len(view.Displays) != 1 || view.Displays[0].Connected {
			t.Fatalf("restored: %+v, %v", view, err)
		}
		devices = []turzx.Device{{ID: id, Name: "TURZX 3.5-inch (USB35INC)"}}
		view, err = reloaded.Get()
		if err != nil || !view.Displays[0].Connected || view.DisplayID != id {
			t.Fatal("reconnected identity changed")
		}
	}
}

func TestCompactSettingValidationAndFailedWriteDoNotChangeSettings(t *testing.T) {
	devices := []turzx.Device{}
	s, path := newService(t, &devices)
	for _, interval := range []int{5, 300} {
		if _, err := s.Save(SaveRequest{Source: "Local", RotationIntervalSeconds: &interval}); err != nil {
			t.Fatal(err)
		}
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	changes := 0
	s.OnDisplaySaved = func() { changes++ }
	for _, interval := range []int{0, 4, 301} {
		if fields := fieldErrors(t, mustFail(t, s, SaveRequest{Source: "Local", RotationIntervalSeconds: &interval})); fields["rotationIntervalSeconds"] == "" {
			t.Fatal("no interval error")
		}
	}
	invalid := "UpsideDown"
	if fields := fieldErrors(t, mustFail(t, s, SaveRequest{Source: "Local", Orientation: &invalid})); fields["orientation"] == "" {
		t.Fatal("no orientation error")
	}
	if err := os.Chmod(path, 0444); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(path, 0600)
	orientation := "ReversePortrait"
	if _, err := s.Save(SaveRequest{Source: "Local", Orientation: &orientation}); err == nil {
		t.Fatal("read-only replacement succeeded")
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(before, after) || changes != 0 {
		t.Fatalf("failed save changed settings/callbacks: %v, %d", err, changes)
	}
	view, err := s.Get()
	if err != nil || view.Orientation != "Landscape" || view.RotationIntervalSeconds != 300 {
		t.Fatalf("failed save changed view: %+v, %v", view, err)
	}
}
