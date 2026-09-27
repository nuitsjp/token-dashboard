package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"log/slog"
	"os"
	"path/filepath"
	"time"

	"token-monitor-turzx/internal/updates"
)

// mockUpdateDelay leaves time to open the window before the update appears.
const mockUpdateDelay = 10 * time.Second

// mockRelease stands in for GitHub Releases in dev:mock (stages 2 and 3; removed in stage 4):
// a signed v0.2.0 in a local folder, whose installer is never executed.
func mockRelease(dir string, cfg *updates.Config, logger *slog.Logger) (func(string) error, error) {
	pub, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, err
	}
	source := filepath.Join(dir, "mock-release")
	if err = os.MkdirAll(source, 0700); err != nil {
		return nil, err
	}
	installer := []byte("mock installer - never executed")
	hash := sha256.Sum256(installer)
	m := updates.Manifest{AppID: cfg.AppID, Version: "0.2.0", OS: "windows", Arch: cfg.Arch, Filename: "token-monitor-turzx-setup.exe", Size: int64(len(installer)), SHA256: hex.EncodeToString(hash[:])}
	if err = os.WriteFile(filepath.Join(source, m.Filename), installer, 0600); err != nil {
		return nil, err
	}
	signed, err := updates.Sign(m, key)
	if err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(source, updates.ManifestName), signed, 0600); err != nil {
		return nil, err
	}
	cfg.Source, cfg.PublicKey = source, base64.StdEncoding.EncodeToString(pub)
	return func(path string) error {
		logger.Info("mock_installer_not_executed", "file", filepath.Base(path))
		return nil
	}, nil
}
