package updates

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"token-monitor-turzx/internal/appstate"
	"token-monitor-turzx/internal/fault"
)

func fixture(t *testing.T, version string) (Config, ed25519.PrivateKey, Manifest) {
	t.Helper()
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	data := []byte("test installer bytes - never executed")
	hash := sha256.Sum256(data)
	m := Manifest{AppID: "test.app", Version: version, OS: "windows", Arch: "amd64", Filename: "setup.exe", Size: int64(len(data)), SHA256: hex.EncodeToString(hash[:])}
	if err = os.WriteFile(filepath.Join(dir, m.Filename), data, 0600); err != nil {
		t.Fatal(err)
	}
	b, err := Sign(m, key)
	if err != nil {
		t.Fatal(err)
	}
	if err = os.WriteFile(filepath.Join(dir, ManifestName), b, 0600); err != nil {
		t.Fatal(err)
	}
	return Config{AppID: m.AppID, Version: "0.1.0", Arch: m.Arch, Source: dir, PublicKey: base64.StdEncoding.EncodeToString(pub), CacheDir: t.TempDir(), Enabled: true}, key, m
}
func testUpdater(cfg Config, state *appstate.State, launch func(string) error, approveQuit func()) *Service {
	return New(cfg, state, slog.New(slog.NewTextHandler(io.Discard, nil)), func(Status) {}, launch, approveQuit)
}
func TestVerifiedFolderUpdate(t *testing.T) {
	cfg, _, _ := fixture(t, "0.2.0")
	state := &appstate.State{}
	launched, approved := false, false
	s := testUpdater(cfg, state, func(path string) error {
		launched = true
		if _, err := os.Stat(path); err != nil {
			t.Error(err)
		}
		return nil
	}, func() {
		if !launched {
			t.Fatal("quit approved before installer launch")
		}
		approved = true
	})
	status, err := s.check(context.Background())
	if err != nil || !status.Available {
		t.Fatalf("check %#v %v", status, err)
	}
	status, err = s.download(context.Background())
	if err != nil || status.Phase != "ready" {
		t.Fatalf("download %#v %v", status, err)
	}
	if err = s.Apply(); err != nil {
		t.Fatal(err)
	}
	if !launched || !approved {
		t.Fatal("update did not hand off")
	}
	if _, err := state.Begin(); err == nil {
		t.Fatal("work accepted after update exit")
	}
}
func TestTamperingAndWrongKey(t *testing.T) {
	cfg, key, m := fixture(t, "0.2.0")
	b, _ := Sign(m, key)
	var envelope Envelope
	json.Unmarshal(b, &envelope)
	payload, _ := base64.StdEncoding.DecodeString(envelope.Payload)
	payload[5] ^= 1
	envelope.Payload = base64.StdEncoding.EncodeToString(payload)
	tampered, _ := json.Marshal(envelope)
	if _, err := Verify(tampered, key.Public().(ed25519.PublicKey)); err == nil {
		t.Fatal("tampered payload accepted")
	}
	wrong, _, _ := ed25519.GenerateKey(rand.Reader)
	cfg.PublicKey = base64.StdEncoding.EncodeToString(wrong)
	if _, err := testUpdater(cfg, &appstate.State{}, nil, func() {}).check(context.Background()); err == nil {
		t.Fatal("wrong key accepted")
	}
}
func TestWrongTargetAndOlderVersion(t *testing.T) {
	cfg, _, _ := fixture(t, "0.0.1")
	s := testUpdater(cfg, &appstate.State{}, nil, func() {})
	status, err := s.check(context.Background())
	if err != nil || status.Available {
		t.Fatalf("downgrade offered %#v %v", status, err)
	}
	if _, err = s.download(context.Background()); err == nil {
		t.Fatal("downgrade downloadable")
	}
	cfg.AppID = "different.app"
	if _, err = testUpdater(cfg, &appstate.State{}, nil, func() {}).check(context.Background()); err == nil {
		t.Fatal("wrong target accepted")
	}
}
func TestCorruptDownloadAndRecheckBeforeApply(t *testing.T) {
	cfg, _, m := fixture(t, "0.2.0")
	s := testUpdater(cfg, &appstate.State{}, func(string) error { t.Fatal("untrusted file executed"); return nil }, func() {})
	if _, err := s.check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.download(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.staged, []byte("changed"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := s.Apply(); err == nil {
		t.Fatal("mutable staged file trusted")
	}
	if err := os.WriteFile(filepath.Join(cfg.Source, m.Filename), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err := s.download(context.Background()); err == nil {
		t.Fatal("corrupt download accepted")
	}
}
func TestLaunchFailureAllowsContinuedUse(t *testing.T) {
	cfg, _, _ := fixture(t, "0.2.0")
	state := &appstate.State{}
	s := testUpdater(cfg, state, func(string) error { return errors.New("blocked") }, func() { t.Fatal("quit approved after failed launch") })
	if _, err := s.check(context.Background()); err != nil {
		t.Fatal(err)
	}
	if _, err := s.download(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := s.Apply(); err == nil {
		t.Fatal("expected launch failure")
	}
	done, err := state.Begin()
	if err != nil {
		t.Fatal("application left blocked")
	}
	done()
}
func TestVersionAndPathValidation(t *testing.T) {
	for _, v := range []string{"1", "1.0.0-beta", "01.2.3", "65536.0.0", "1.0.-1"} {
		if _, err := CompareVersion(v, "0.1.0"); err == nil {
			t.Fatalf("invalid version %q accepted", v)
		}
	}
	if n, err := CompareVersion("1.10.0", "1.9.9"); err != nil || n != 1 {
		t.Fatal("not numeric comparison")
	}
	_, key, m := fixture(t, "0.2.0")
	for _, name := range []string{"../setup.exe", `dir\setup.exe`, "https:setup.exe"} {
		m.Filename = name
		if _, err := Sign(m, key); err == nil {
			t.Fatal("unsafe filename accepted")
		}
	}
}
func TestCancelledDownloadCleansStage(t *testing.T) {
	cfg, _, _ := fixture(t, "0.2.0")
	s := testUpdater(cfg, &appstate.State{}, nil, func() {})
	s.check(context.Background())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := s.download(ctx); err == nil {
		t.Fatal("cancellation ignored")
	}
	entries, _ := os.ReadDir(cfg.CacheDir)
	if len(entries) != 0 {
		t.Fatal("failed download retained")
	}
}

func TestStartupPublishesItsFinalStatus(t *testing.T) {
	for _, version := range []string{"0.0.1", "0.1.0", "0.2.0"} {
		t.Run(version, func(t *testing.T) {
			cfg, _, _ := fixture(t, version)
			s := testUpdater(cfg, &appstate.State{}, nil, func() {})
			var phases []string
			var notified Status
			s.onStatus = func(status Status) {
				if !reflect.DeepEqual(status, s.GetStatus()) {
					t.Fatal("notification and GetStatus disagree")
				}
				phases = append(phases, status.Phase)
				notified = status
			}
			Run(context.Background(), s)
			if len(phases) == 0 || phases[0] != PhaseChecking || !slices.Contains(phases, PhaseChecked) {
				t.Fatalf("missing check status: %v", phases)
			}
			newer := version == "0.2.0"
			wantPhase := PhaseChecked
			if newer {
				wantPhase = PhaseReady
			}
			if notified.Phase != wantPhase || notified.Available != newer || notified.Version != version || notified.ApplyError != nil {
				t.Fatalf("final notification = %+v", notified)
			}
			if !newer && s.staged != "" {
				t.Fatal("an installer was staged without a newer version")
			}
		})
	}
}

func TestApplyVerificationFailuresAreSharedStatus(t *testing.T) {
	for _, change := range []string{"missing", "changed", "unreadable"} {
		t.Run(change, func(t *testing.T) {
			cfg, _, _ := fixture(t, "0.2.0")
			state := &appstate.State{}
			s := testUpdater(cfg, state, func(string) error {
				t.Fatal("unverified installer launched")
				return nil
			}, func() { t.Fatal("quit approved for unverified installer") })
			Run(context.Background(), s)
			if s.GetStatus().Phase != PhaseReady {
				t.Fatal("fixture update was not ready")
			}
			if err := os.Remove(s.staged); err != nil {
				t.Fatal(err)
			}
			switch change {
			case "changed":
				if err := os.WriteFile(s.staged, []byte("changed"), 0600); err != nil {
					t.Fatal(err)
				}
			case "unreadable":
				// A directory can never be read as the staged executable on any supported OS.
				if err := os.Mkdir(s.staged, 0700); err != nil {
					t.Fatal(err)
				}
			}
			var notified Status
			s.onStatus = func(status Status) { notified = status }
			err := s.Apply()
			if err == nil || fault.Public(err).Code != "UPDATE_UNTRUSTED" {
				t.Fatalf("apply error = %v", err)
			}
			if notified.Phase != PhaseUntrusted || !notified.Available || notified.ApplyError == nil || notified.ApplyError.Code != "UPDATE_UNTRUSTED" || !reflect.DeepEqual(notified, s.GetStatus()) {
				t.Fatalf("shared failure status = %+v", notified)
			}
			done, err := state.Begin()
			if err != nil {
				t.Fatal("failed verification left the application closing")
			}
			done()
		})
	}
}

func TestLaunchFailureCanBeRetriedAndClearsSharedError(t *testing.T) {
	cfg, _, _ := fixture(t, "0.2.0")
	state := &appstate.State{}
	fail := true
	approved := false
	s := testUpdater(cfg, state, func(string) error {
		if fail {
			return errors.New("private installer path")
		}
		return nil
	}, func() { approved = true })
	Run(context.Background(), s)
	var notified Status
	s.onStatus = func(status Status) { notified = status }
	if err := s.Apply(); err == nil {
		t.Fatal("expected launch failure")
	}
	if approved || notified.Phase != PhaseReady || notified.ApplyError == nil || notified.ApplyError.Code != "INTERNAL" || !reflect.DeepEqual(notified, s.GetStatus()) {
		t.Fatalf("shared launch failure = %+v, approved = %v", notified, approved)
	}
	public, err := json.Marshal(notified)
	if err != nil || strings.Contains(string(public), "private installer path") {
		t.Fatalf("unsafe public failure = %s, %v", public, err)
	}
	fail = false
	if err := s.Apply(); err != nil {
		t.Fatal(err)
	}
	if !approved || notified.Phase != PhaseHandedOff || notified.ApplyError != nil || !reflect.DeepEqual(notified, s.GetStatus()) {
		t.Fatalf("handoff status = %+v, approved = %v", notified, approved)
	}
}

func TestApplyRejectionIsSharedWithoutAbortingAnExistingExit(t *testing.T) {
	for _, code := range []string{"BUSY", "CLOSING"} {
		t.Run(code, func(t *testing.T) {
			cfg, _, _ := fixture(t, "0.2.0")
			state := &appstate.State{}
			s := testUpdater(cfg, state, func(string) error {
				t.Fatal("rejected update launched")
				return nil
			}, func() { t.Fatal("rejected update approved quit") })
			Run(context.Background(), s)
			if code == "BUSY" {
				done, err := state.Begin()
				if err != nil {
					t.Fatal(err)
				}
				defer done()
			} else if err := state.PrepareExit(); err != nil {
				t.Fatal(err)
			}
			var notified Status
			s.onStatus = func(status Status) { notified = status }
			if err := s.Apply(); err == nil || fault.Public(err).Code != code {
				t.Fatalf("rejection = %v", err)
			}
			if notified.Phase != PhaseReady || notified.ApplyError == nil || notified.ApplyError.Code != code || !reflect.DeepEqual(notified, s.GetStatus()) {
				t.Fatalf("shared rejection = %+v", notified)
			}
			if code == "CLOSING" {
				if _, err := state.Begin(); err == nil {
					t.Fatal("rejected update cancelled an existing exit")
				}
			}
		})
	}
}

func TestConcurrentApplyRejectionDoesNotOutliveSuccessfulHandoff(t *testing.T) {
	cfg, _, _ := fixture(t, "0.2.0")
	started, finish := make(chan struct{}), make(chan struct{})
	s := testUpdater(cfg, &appstate.State{}, func(string) error {
		close(started)
		<-finish
		return nil
	}, func() {})
	Run(context.Background(), s)
	done := make(chan error, 1)
	go func() { done <- s.Apply() }()
	<-started
	err := s.Apply()
	status := s.GetStatus()
	close(finish)
	firstErr := <-done
	if err == nil || fault.Public(err).Code != "BUSY" || status.ApplyError == nil || status.ApplyError.Code != "BUSY" {
		t.Fatalf("concurrent rejection = %v, status = %+v", err, status)
	}
	if firstErr != nil {
		t.Fatal(firstErr)
	}
	if status := s.GetStatus(); status.Phase != PhaseHandedOff || status.ApplyError != nil {
		t.Fatalf("successful handoff retained an error: %+v", status)
	}
}
