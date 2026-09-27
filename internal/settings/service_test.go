//go:build windows

package settings

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
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
	if _, err := s.Save(SaveRequest{Source: "Hub", URL: "https://hub.example.com", Token: "secret", DisplayID: second}); err != nil {
		t.Fatal(err)
	}
	devices = devices[:1]
	view, err := s.Get()
	if err != nil {
		t.Fatal(err)
	}
	if view.DisplayID != second || len(view.Displays) != 2 {
		t.Fatalf("view = %+v", view)
	}
	if d := view.Displays[1]; d.DeviceID != second || d.Name != "TURZX1.0 (8F21C4D0)" || d.Connected {
		t.Fatalf("unplugged display = %+v", d)
	}
	// Saving again while unplugged keeps the selection and its name.
	if _, err := s.Save(SaveRequest{Source: "Hub", URL: "https://hub.example.com", DisplayID: second}); err != nil {
		t.Fatal(err)
	}
	if view, _ := s.Get(); view.Displays[1].Name != "TURZX1.0 (8F21C4D0)" {
		t.Fatalf("view = %+v", view)
	}
}

func TestSaveRejectsUnknownDisplay(t *testing.T) {
	devices := []turzx.Device{{ID: first, Name: "TURZX1.0 (633A6E01)"}}
	s, path := newService(t, &devices)
	_, err := s.Save(SaveRequest{Source: "Hub", URL: "https://hub.example.com", Token: "secret", DisplayID: second})
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
		if fieldErrors(t, mustFail(t, s, SaveRequest{Source: "Hub", URL: url, Token: "secret"}))["url"] == "" {
			t.Errorf("accepted %q", url)
		}
	}
	if fieldErrors(t, mustFail(t, s, SaveRequest{Source: "Hub", URL: "https://hub"}))["token"] == "" {
		t.Error("accepted a missing token")
	}
	if fieldErrors(t, mustFail(t, s, SaveRequest{Source: "Hub", URL: "https://hub", Token: "a\nb"}))["token"] == "" {
		t.Error("accepted a control character")
	}
	view, err := s.Save(SaveRequest{Source: "Hub", URL: " http://127.0.0.1:8080/ ", Token: "secret"})
	if err != nil || view.URL != "http://127.0.0.1:8080" {
		t.Fatalf("view = %+v, %v", view, err)
	}
}

func mustFail(t *testing.T, s *Service, req SaveRequest) error {
	t.Helper()
	_, err := s.Save(req)
	if err == nil {
		t.Fatalf("Save(%+v) succeeded", req)
	}
	return err
}

func TestSaveEncryptsAndKeepsTokenWhenBlank(t *testing.T) {
	devices := []turzx.Device{}
	s, path := newService(t, &devices)
	if _, err := s.Save(SaveRequest{Source: "Hub", URL: "https://hub.example.com", Token: "first-secret"}); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(SaveRequest{Source: "Hub", URL: "https://other.example.com"}); err != nil {
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
	if _, err := s.Save(SaveRequest{Source: "Hub", URL: "https://hub", Token: "secret"}); !errors.As(err, &public) || public.Code != "SETTINGS_UNREADABLE" {
		t.Fatalf("err = %v", err)
	}
	if data, _ := os.ReadFile(path); !strings.Contains(string(data), "bm90LWRwYXBp") {
		t.Fatal("unreadable settings were overwritten")
	}
}
