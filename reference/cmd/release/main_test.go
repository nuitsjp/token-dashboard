package main

import (
	"crypto/ed25519"
	"encoding/base64"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"wailstemplate/internal/updates"
)

func TestKeygenAndSignedManifest(t *testing.T) {
	dir := t.TempDir()
	keyPath := filepath.Join(dir, "private.txt")
	if err := run([]string{"keygen", "-out", keyPath}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"keygen", "-out", keyPath}); err == nil {
		t.Fatal("existing private key overwritten")
	}
	path := filepath.Join(dir, "setup.exe")
	if err := os.WriteFile(path, []byte("test bytes, not executable"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"manifest", "-key", keyPath, "-installer", path, "-app-id", "test.app", "-version", "1.0.0", "-out", dir}); err != nil {
		t.Fatal(err)
	}
	keyBytes, _ := os.ReadFile(keyPath)
	key, _ := base64.StdEncoding.DecodeString(strings.TrimSpace(string(keyBytes)))
	data, _ := os.ReadFile(filepath.Join(dir, updates.ManifestName))
	manifest, err := updates.Verify(data, ed25519.PrivateKey(key).Public().(ed25519.PublicKey))
	if err != nil || manifest.Version != "1.0.0" {
		t.Fatalf("manifest %v %#v", err, manifest)
	}
}
