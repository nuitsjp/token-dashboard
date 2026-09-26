package updates

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"wailstemplate/internal/appstate"
	"wailstemplate/internal/fault"
)

const ProgressEvent = "updates:progress"

type Config struct {
	AppID, Version, Arch, Source, PublicKey, CacheDir string
	Enabled                                           bool
}
type Status struct {
	Configured bool   `json:"configured"`
	Available  bool   `json:"available"`
	Version    string `json:"version"`
	Notes      string `json:"notes"`
	Phase      string `json:"phase"`
	Downloaded int64  `json:"downloaded"`
	Total      int64  `json:"total"`
}
type Service struct {
	cfg         Config
	mu          sync.Mutex
	busy        bool
	manifest    *Manifest
	staged      string
	status      Status
	client      *http.Client
	state       *appstate.State
	logger      *slog.Logger
	publish     func(string, any)
	launch      func(string) error
	approveQuit func()
}

func New(cfg Config, state *appstate.State, logger *slog.Logger, publish func(string, any), launch func(string) error, approveQuit func()) *Service {
	return &Service{cfg: cfg, state: state, logger: logger, publish: publish, launch: launch, approveQuit: approveQuit, client: newHTTPClient(), status: Status{Configured: cfg.Source != "" && cfg.PublicKey != "", Phase: "idle"}}
}
func (s *Service) GetStatus() Status { s.mu.Lock(); defer s.mu.Unlock(); return s.status }
func (s *Service) acquire() (func(), error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.busy {
		return nil, fault.New("BUSY", "別の更新操作が実行中です。")
	}
	s.busy = true
	return func() { s.mu.Lock(); s.busy = false; s.mu.Unlock() }, nil
}
func (s *Service) report(phase string, downloaded, total int64) {
	s.mu.Lock()
	s.status.Phase = phase
	s.status.Downloaded = downloaded
	s.status.Total = total
	status := s.status
	s.mu.Unlock()
	s.publish(ProgressEvent, status)
}
func (s *Service) Check(ctx context.Context) (status Status, err error) {
	defer func() { err = fault.Boundary(s.logger, "updates.check", err) }()
	release, err := s.acquire()
	if err != nil {
		return status, err
	}
	defer release()
	if s.cfg.Source == "" || s.cfg.PublicKey == "" {
		return status, fault.New("UPDATE_NOT_CONFIGURED", "更新元と公開鍵が設定されていません。")
	}
	s.report("checking", 0, 0)
	defer func() {
		if err != nil {
			s.report("failed", 0, 0)
		}
	}()
	key, err := base64.StdEncoding.DecodeString(s.cfg.PublicKey)
	if err != nil || len(key) != ed25519.PublicKeySize {
		return status, fault.New("UPDATE_CONFIGURATION", "更新用公開鍵が不正です。")
	}
	reader, err := openSource(ctx, s.client, s.cfg.Source, ManifestName)
	if err != nil {
		return status, err
	}
	defer reader.Close()
	b, err := io.ReadAll(io.LimitReader(reader, maxManifestSize+1))
	if err != nil {
		return status, err
	}
	m, err := Verify(b, ed25519.PublicKey(key))
	if err != nil {
		s.logger.Warn("update_verification_failed", "cause", err)
		return status, fault.New("UPDATE_UNTRUSTED", "更新情報の署名を確認できませんでした。")
	}
	if m.AppID != s.cfg.AppID || m.Arch != s.cfg.Arch {
		return status, fault.New("UPDATE_TARGET", "更新ファイルの対象アプリまたはCPUが一致しません。")
	}
	cmp, err := CompareVersion(m.Version, s.cfg.Version)
	if err != nil {
		return status, err
	}
	s.mu.Lock()
	s.status = Status{Configured: true, Available: cmp > 0, Version: m.Version, Notes: m.Notes, Phase: "checked"}
	s.manifest = &m
	// A new check invalidates any earlier staged installer.
	oldStage := s.staged
	s.staged = ""
	status = s.status
	s.mu.Unlock()
	if oldStage != "" {
		os.Remove(oldStage)
		os.Remove(filepath.Dir(oldStage))
	}
	return status, nil
}
func (s *Service) Download(ctx context.Context) (status Status, err error) {
	defer func() { err = fault.Boundary(s.logger, "updates.download", err) }()
	release, err := s.acquire()
	if err != nil {
		return status, err
	}
	defer release()
	done, err := s.state.Begin()
	if err != nil {
		return status, err
	}
	defer done()
	s.mu.Lock()
	m := s.manifest
	available := s.status.Available
	s.mu.Unlock()
	if m == nil || !available {
		return status, fault.New("UPDATE_NOT_CHECKED", "先に新版の有無を確認してください。")
	}
	if err = os.MkdirAll(s.cfg.CacheDir, 0700); err != nil {
		return status, err
	}
	stageDir, err := os.MkdirTemp(s.cfg.CacheDir, "update-")
	if err != nil {
		return status, err
	}
	path := filepath.Join(stageDir, m.Filename)
	defer func() {
		if err != nil {
			os.RemoveAll(stageDir)
			s.report("failed", 0, m.Size)
		}
	}()
	source, err := openSource(ctx, s.client, s.cfg.Source, m.Filename)
	if err != nil {
		return status, err
	}
	defer source.Close()
	target, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if err != nil {
		return status, err
	}
	h := sha256.New()
	var copied int64
	writer := io.MultiWriter(target, h)
	limited := io.LimitReader(source, m.Size+1)
	buf := make([]byte, 64*1024)
	s.report("downloading", 0, m.Size)
	for {
		if err = ctx.Err(); err != nil {
			break
		}
		n, readErr := limited.Read(buf)
		if n > 0 {
			var written int
			written, err = writer.Write(buf[:n])
			copied += int64(written)
			if err != nil {
				break
			}
			if written != n {
				err = io.ErrShortWrite
				break
			}
			s.report("downloading", copied, m.Size)
		}
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			err = readErr
			break
		}
	}
	if err == nil {
		err = target.Sync()
	}
	if closeErr := target.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return status, err
	}
	if copied != m.Size || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), m.SHA256) {
		return status, fault.New("UPDATE_UNTRUSTED", "更新ファイルのサイズまたはハッシュが一致しません。")
	}
	s.mu.Lock()
	previous := s.staged
	s.staged = path
	s.mu.Unlock()
	if previous != "" {
		os.Remove(previous)
		os.Remove(filepath.Dir(previous))
	}
	s.report("ready", copied, m.Size)
	return s.GetStatus(), nil
}
func (s *Service) Apply() (err error) {
	defer func() { err = fault.Boundary(s.logger, "updates.apply", err) }()
	release, err := s.acquire()
	if err != nil {
		return err
	}
	defer release()
	if !s.cfg.Enabled {
		return fault.New("DESKTOP_ONLY", "更新の適用はWindowsデスクトップ版で実行してください。")
	}
	s.mu.Lock()
	path, m := s.staged, s.manifest
	s.mu.Unlock()
	if path == "" || m == nil {
		return fault.New("UPDATE_NOT_READY", "更新ファイルを先に取得してください。")
	}
	if err = s.state.PrepareExit(); err != nil {
		return err
	}
	defer func() {
		if err != nil {
			s.state.AbortExit()
		}
	}()
	// Check again immediately before execution; a successful download is not a
	// permanent trust decision about a mutable file on disk.
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	h := sha256.New()
	n, err := io.Copy(h, io.LimitReader(f, m.Size+1))
	f.Close()
	if err != nil {
		return err
	}
	if n != m.Size || !strings.EqualFold(hex.EncodeToString(h.Sum(nil)), m.SHA256) {
		return fault.New("UPDATE_UNTRUSTED", "適用前の再検証に失敗しました。")
	}
	if s.launch == nil {
		return errors.New("installer launcher is not configured")
	}
	if err = s.launch(path); err != nil {
		return fmt.Errorf("could not launch installer: %w", err)
	}
	s.report("handed-off", m.Size, m.Size)
	s.approveQuit()
	return nil
}
