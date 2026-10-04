//go:build windows

package settings

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"token-monitor-turzx/internal/fault"
	"token-monitor-turzx/internal/turzx"
)

const (
	first  = `USB\VID_1CBE&PID_0092\633A6E01A48A0706`
	second = `USB\VID_1CBE&PID_0092\8F21C4D09B3E5A17`
)

func newService(t *testing.T, devices *[]turzx.Device) (*Service, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "settings.json")
	list := func() ([]turzx.Device, error) { return *devices, nil }
	return New(path, "test.app", list, slog.New(slog.NewTextHandler(io.Discard, nil))), path
}

func fieldErrors(t *testing.T, err error) map[string]string {
	t.Helper()
	var public *fault.Error
	if !errors.As(err, &public) || public.Code != "VALIDATION" {
		t.Fatalf("want validation error, got %v", err)
	}
	return public.FieldErrors
}

func TestSaveKeepsSelectedDisplayWhileUnplugged(t *testing.T) {
	devices := []turzx.Device{{ID: first, Name: "TURZX1.0 (633A6E01)"}, {ID: second, Name: "TURZX1.0 (8F21C4D0)"}}
	s, _ := newService(t, &devices)
	if _, err := s.SetDisplay(second); err != nil {
		t.Fatal(err)
	}
	devices = devices[:1]
	view, err := s.Get()
	if err != nil {
		t.Fatal(err)
	}
	displays, err := s.GetDisplays()
	if err != nil {
		t.Fatal(err)
	}
	if view.DisplayID != second || len(displays) != 2 {
		t.Fatalf("view = %+v", view)
	}
	if d := displays[1]; d.DeviceID != second || d.Name != "TURZX1.0 (8F21C4D0)" || d.Connected {
		t.Fatalf("unplugged display = %+v", d)
	}
	if target, err := DisplayTarget(s); err != nil || target != second {
		t.Fatalf("unplugged target = %q, %v", target, err)
	}
	// Saving again while unplugged keeps the selection and its name.
	if _, err := s.SetDisplay(second); err != nil {
		t.Fatal(err)
	}
	if displays, _ := s.GetDisplays(); displays[1].Name != "TURZX1.0 (8F21C4D0)" {
		t.Fatalf("displays = %+v", displays)
	}
}

func TestDisplayTargetAutomaticUsesFirstConnected(t *testing.T) {
	devices := []turzx.Device{{ID: first, Name: "TURZX1.0 (633A6E01)"}, {ID: second, Name: "TURZX1.0 (8F21C4D0)"}}
	s, _ := newService(t, &devices)
	if target, err := DisplayTarget(s); err != nil || target != first {
		t.Fatalf("automatic target = %q, %v", target, err)
	}
}

func TestSaveLocalWithoutHubConnectionAndReload(t *testing.T) {
	devices := []turzx.Device{{ID: first, Name: "TURZX1.0 (633A6E01)"}}
	s, path := newService(t, &devices)

	if _, err := s.SetDisplay(first); err != nil {
		t.Fatal(err)
	}
	view, err := s.SaveConnection(ConnectionRequest{Source: "Local"})
	if err != nil {
		t.Fatal(err)
	}
	if view.Source != "Local" || view.URL != "" || view.TokenSet || view.DisplayID != first {
		t.Fatalf("view = %+v", view)
	}

	reloaded := New(path, "test.app", func() ([]turzx.Device, error) { return devices, nil }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	view, err = reloaded.Get()
	if err != nil {
		t.Fatal(err)
	}
	if view.Source != "Local" || view.DisplayID != first {
		t.Fatalf("reloaded view = %+v", view)
	}
	if source, err := Source(reloaded); err != nil || source != "Local" {
		t.Fatalf("source = %q, %v", source, err)
	}
}

func TestSaveLocalKeepsSavedConnection(t *testing.T) {
	devices := []turzx.Device{}
	s, _ := newService(t, &devices)
	if _, err := s.SaveConnection(ConnectionRequest{Source: "Hub", URL: "https://hub.example.com", Token: "secret"}); err != nil {
		t.Fatal(err)
	}
	saved, _, err := s.read()
	if err != nil {
		t.Fatal(err)
	}
	if saved.Connection == "" {
		t.Fatal("Hub connection was not saved")
	}

	view, err := s.SaveConnection(ConnectionRequest{Source: "Local"})
	if err != nil {
		t.Fatal(err)
	}
	if view.Source != "Local" || view.URL != "https://hub.example.com" || !view.TokenSet {
		t.Fatalf("view = %+v", view)
	}
	after, _, err := s.read()
	if err != nil {
		t.Fatal(err)
	}
	if after.Source != "Local" || after.Connection != saved.Connection {
		t.Fatalf("saved file = %+v, before = %+v", after, saved)
	}
}

func TestSaveRejectsUnknownDisplay(t *testing.T) {
	devices := []turzx.Device{{ID: first, Name: "TURZX1.0 (633A6E01)"}}
	s, path := newService(t, &devices)
	_, err := s.SetDisplay(second)
	if fieldErrors(t, err)["displayID"] == "" {
		t.Fatalf("err = %v", err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("rejected input was saved")
	}
}

func TestSaveValidatesURLAndToken(t *testing.T) {
	devices := []turzx.Device{}
	s, _ := newService(t, &devices)
	for _, url := range []string{"", "ftp://hub", "https://", "https://user@hub", "https://hub/api", "https://hub?x=1", "https://hub#x"} {
		if fieldErrors(t, mustFail(t, s, ConnectionRequest{Source: "Hub", URL: url, Token: "secret"}))["url"] == "" {
			t.Errorf("accepted %q", url)
		}
	}
	if fieldErrors(t, mustFail(t, s, ConnectionRequest{Source: "Hub", URL: "https://hub"}))["token"] == "" {
		t.Error("accepted a missing token")
	}
	if fieldErrors(t, mustFail(t, s, ConnectionRequest{Source: "Hub", URL: "https://hub", Token: "a\nb"}))["token"] == "" {
		t.Error("accepted a control character")
	}
	view, err := s.SaveConnection(ConnectionRequest{Source: "Hub", URL: " http://127.0.0.1:8080/ ", Token: "secret"})
	if err != nil || view.URL != "http://127.0.0.1:8080" {
		t.Fatalf("view = %+v, %v", view, err)
	}
}

func mustFail(t *testing.T, s *Service, req ConnectionRequest) error {
	t.Helper()
	_, err := s.SaveConnection(req)
	if err == nil {
		t.Fatalf("Save(%+v) succeeded", req)
	}
	return err
}

func TestSaveEncryptsAndKeepsTokenWhenBlank(t *testing.T) {
	devices := []turzx.Device{}
	s, path := newService(t, &devices)
	if _, err := s.SaveConnection(ConnectionRequest{Source: "Hub", URL: "https://hub.example.com", Token: "first-secret"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SaveConnection(ConnectionRequest{Source: "Hub", URL: "https://other.example.com"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, plain := range []string{"example.com", "first-secret"} {
		if strings.Contains(string(data), plain) {
			t.Fatalf("settings file contains %q", plain)
		}
	}
	_, conn, err := s.read()
	if err != nil || conn != (connection{URL: "https://other.example.com", Token: "first-secret"}) {
		t.Fatalf("connection = %+v, %v", conn, err)
	}
	if view, _ := s.Get(); !view.TokenSet || view.DisplayID != "" {
		t.Fatalf("view = %+v", view)
	}
}

func TestUnreadableFileIsNotOverwritten(t *testing.T) {
	devices := []turzx.Device{}
	s, path := newService(t, &devices)
	if err := os.WriteFile(path, []byte(`{"connection":"bm90LWRwYXBp","displayID":"","displayName":""}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var public *fault.Error
	if _, err := s.SaveConnection(ConnectionRequest{Source: "Hub", URL: "https://hub", Token: "secret"}); !errors.As(err, &public) || public.Code != "SETTINGS_UNREADABLE" {
		t.Fatalf("err = %v", err)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "bm90LWRwYXBp") {
		t.Fatal("unreadable settings were overwritten")
	}
}

func TestLimitStyleDefaultsToGaugesAndIsSavedInTheFile(t *testing.T) {
	devices := []turzx.Device{}
	s, path := newService(t, &devices)
	if view, err := s.Get(); err != nil || view.LimitStyle != "Gauges" || LimitStyle(s) != "Gauges" {
		t.Fatalf("default view = %+v, %v", view, err)
	}
	if _, err := s.SetLimitStyle("Bars"); err != nil {
		t.Fatal(err)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), `"limitStyle": "Bars"`) {
		t.Fatalf("settings file = %s", data)
	}
	if view, _ := s.Get(); view.LimitStyle != "Bars" || LimitStyle(s) != "Bars" {
		t.Fatalf("saved view = %+v", view)
	}
	// Saving a connection keeps the style, and Gauges can be chosen again.
	if view, err := s.SaveConnection(ConnectionRequest{Source: "Local"}); err != nil || view.LimitStyle != "Bars" {
		t.Fatalf("empty style view = %+v, %v", view, err)
	}
	if style, err := s.SetLimitStyle("Gauges"); err != nil || style != "Gauges" {
		t.Fatalf("gauges style = %q, %v", style, err)
	}
	_, err := s.SetLimitStyle("Dials")
	if got := fieldErrors(t, err)["limitStyle"]; got == "" {
		t.Fatal("an unknown style was accepted")
	}
	if LimitStyle(s) != "Gauges" {
		t.Fatalf("style after a rejected save = %q", LimitStyle(s))
	}
}

func TestOnlyANewSourceOrConnectionRestartsReading(t *testing.T) {
	devices := []turzx.Device{{ID: first, Name: "TURZX1.0 (633A6E01)"}}
	s, _ := newService(t, &devices)
	var sources, styles int
	s.OnSaved = func() { sources++ }
	s.OnStyleSaved = func() { styles++ }
	save := func(req ConnectionRequest) {
		t.Helper()
		if _, err := s.SaveConnection(req); err != nil {
			t.Fatal(err)
		}
	}
	hub := ConnectionRequest{Source: "Hub", URL: "https://hub.example.com", Token: "secret"}
	save(hub)
	if sources != 1 {
		t.Fatalf("a new source restarted reading %d times", sources)
	}
	// A display or style choice must not interrupt reading, which would blank the image.
	if _, err := s.SetDisplay(first); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetLimitStyle("Bars"); err != nil {
		t.Fatal(err)
	}
	if sources != 1 || styles != 1 {
		t.Fatalf("display and style: restarts = %d, style changes = %d", sources, styles)
	}
	if _, err := s.SetDisplay(""); err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetLimitStyle("Bars"); err != nil {
		t.Fatal(err)
	}
	if sources != 1 || styles != 1 {
		t.Fatalf("same style again: restarts = %d, style changes = %d", sources, styles)
	}
	hub.URL = "https://other.example.com"
	save(hub)
	if sources != 2 {
		t.Fatalf("a new connection restarted reading %d times", sources)
	}
	save(ConnectionRequest{Source: "Local"})
	if sources != 3 {
		t.Fatalf("a new source restarted reading %d times", sources)
	}
}

func TestConcurrentSettingsOperationsKeepEachOthersValues(t *testing.T) {
	devices := []turzx.Device{{ID: first, Name: "TURZX1.0 (633A6E01)"}}
	s, _ := newService(t, &devices)
	if err := SetLimitsShown(s, []string{"hidden-window"}, false); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var workers sync.WaitGroup
	for _, change := range []func() error{
		func() error {
			_, err := s.SaveConnection(ConnectionRequest{Source: "Hub", URL: "https://hub.example.com", Token: "secret"})
			return err
		},
		func() error { _, err := s.SetDisplay(first); return err },
		func() error { _, err := s.SetLimitStyle("Bars"); return err },
	} {
		workers.Add(1)
		go func() {
			defer workers.Done()
			<-start
			if err := change(); err != nil {
				t.Error(err)
			}
		}()
	}
	close(start)
	workers.Wait()
	view, err := s.Get()
	if err != nil {
		t.Fatal(err)
	}
	if view.Source != "Hub" || view.URL != "https://hub.example.com" || !view.TokenSet || view.DisplayID != first || view.LimitStyle != "Bars" {
		t.Fatalf("concurrent changes were lost: %+v", view)
	}
	hidden, err := HiddenLimits(s)
	if err != nil || len(hidden) != 1 || hidden[0] != "hidden-window" {
		t.Fatalf("hidden limits changed: %v, %v", hidden, err)
	}
	saved, _, err := s.read()
	if err != nil || saved.DisplayName != devices[0].Name {
		t.Fatalf("saved display = %+v, %v", saved, err)
	}
}

func TestDeviceEnumerationFailureDoesNotBlockConnectionOrStyle(t *testing.T) {
	devices := []turzx.Device{}
	s, _ := newService(t, &devices)
	listingFailed := errors.New("device enumeration failed")
	s.list = func() ([]turzx.Device, error) { return nil, listingFailed }
	if _, err := s.Get(); err != nil {
		t.Fatalf("settings depended on device enumeration: %v", err)
	}
	if _, err := s.SaveConnection(ConnectionRequest{Source: "Hub", URL: "https://hub.example.com", Token: "secret"}); err != nil {
		t.Fatalf("connection depended on device enumeration: %v", err)
	}
	before, _, err := s.read()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := s.SetLimitStyle("Bars"); err != nil {
		t.Fatalf("style depended on device enumeration: %v", err)
	}
	if _, err := s.SetDisplay(""); err != nil {
		t.Fatalf("Automatic depended on device enumeration: %v", err)
	}
	if _, err := s.GetDisplays(); err == nil {
		t.Fatal("display list ignored enumeration failure")
	}
	if _, err := s.SetDisplay(first); err == nil {
		t.Fatal("selected a device without enumerating it")
	}
	after, _, err := s.read()
	if err != nil || after.Source != "Hub" || after.Connection != before.Connection || after.LimitStyle != "Bars" || after.DisplayID != "" {
		t.Fatalf("unrelated values changed: %+v, %v", after, err)
	}
}

func TestGetDisplaysRefreshesConnectionStatus(t *testing.T) {
	devices := []turzx.Device{}
	s, _ := newService(t, &devices)
	if displays, err := s.GetDisplays(); err != nil || len(displays) != 0 {
		t.Fatalf("initial displays = %v, %v", displays, err)
	}
	devices = []turzx.Device{{ID: first, Name: "TURZX1.0 (633A6E01)"}}
	if _, err := s.SetDisplay(first); err != nil {
		t.Fatal(err)
	}
	if displays, err := s.GetDisplays(); err != nil || len(displays) != 1 || !displays[0].Connected {
		t.Fatalf("connected displays = %v, %v", displays, err)
	}
	devices = nil
	if displays, err := s.GetDisplays(); err != nil || len(displays) != 1 || displays[0].Connected || displays[0].DeviceID != first {
		t.Fatalf("disconnected displays = %v, %v", displays, err)
	}
}
