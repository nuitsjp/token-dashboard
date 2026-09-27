// Package updates verifies signed release metadata and stages an NSIS installer.
package updates

import (
	"crypto/ed25519"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
)

const ManifestName = "update.json"
const maxManifestSize = 128 * 1024
const maxInstallerSize = 512 * 1024 * 1024

type Manifest struct {
	AppID    string `json:"appID"`
	Version  string `json:"version"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Filename string `json:"filename"`
	Size     int64  `json:"size"`
	SHA256   string `json:"sha256"`
	Notes    string `json:"notes"`
}
type Envelope struct {
	Payload   string `json:"payload"`
	Signature string `json:"signature"`
}

// Sign signs the exact payload bytes, so no cross-language JSON canonicaliser
// or unsigned outer metadata is needed. The private key is never distributed.
func Sign(m Manifest, key ed25519.PrivateKey) ([]byte, error) {
	if len(key) != ed25519.PrivateKeySize {
		return nil, errors.New("invalid signing key")
	}
	if err := validateManifest(m); err != nil {
		return nil, err
	}
	payload, err := json.Marshal(m)
	if err != nil {
		return nil, err
	}
	return json.MarshalIndent(Envelope{Payload: base64.StdEncoding.EncodeToString(payload), Signature: base64.StdEncoding.EncodeToString(ed25519.Sign(key, payload))}, "", "  ")
}
func Verify(data []byte, key ed25519.PublicKey) (Manifest, error) {
	var m Manifest
	if len(data) > maxManifestSize || len(key) != ed25519.PublicKeySize {
		return m, errors.New("invalid manifest or public key")
	}
	var e Envelope
	if err := json.Unmarshal(data, &e); err != nil {
		return m, err
	}
	p, err := base64.StdEncoding.DecodeString(e.Payload)
	if err != nil {
		return m, err
	}
	sig, err := base64.StdEncoding.DecodeString(e.Signature)
	if err != nil {
		return m, err
	}
	if !ed25519.Verify(key, p, sig) {
		return m, errors.New("release signature verification failed")
	}
	if err = json.Unmarshal(p, &m); err != nil {
		return m, err
	}
	if err = validateManifest(m); err != nil {
		return Manifest{}, err
	}
	return m, nil
}
func validateManifest(m Manifest) error {
	if m.AppID == "" || m.OS != "windows" || (m.Arch != "amd64" && m.Arch != "arm64") {
		return errors.New("invalid release target")
	}
	if _, err := versionParts(m.Version); err != nil {
		return err
	}
	if m.Filename == "" || len(m.Filename) > 200 || strings.ContainsAny(m.Filename, "/\\:\x00") || !strings.HasSuffix(strings.ToLower(m.Filename), ".exe") {
		return errors.New("installer must be a bare .exe filename")
	}
	if m.Size <= 0 || m.Size > maxInstallerSize {
		return errors.New("invalid installer size")
	}
	hash, err := hex.DecodeString(m.SHA256)
	if err != nil || len(hash) != 32 {
		return errors.New("invalid installer hash")
	}
	return nil
}
func versionParts(s string) ([3]int, error) {
	var result [3]int
	parts := strings.Split(s, ".")
	if len(parts) != 3 {
		return result, fmt.Errorf("version must be major.minor.patch: %q", s)
	}
	for i, part := range parts {
		if part == "" || (len(part) > 1 && part[0] == '0') {
			return result, errors.New("invalid numeric version")
		}
		for _, c := range part {
			if c < '0' || c > '9' {
				return result, errors.New("invalid numeric version")
			}
		}
		n, err := strconv.Atoi(part)
		if err != nil || n > 65535 {
			return result, errors.New("version component exceeds Windows version range")
		}
		result[i] = n
	}
	return result, nil
}
func CompareVersion(a, b string) (int, error) {
	left, err := versionParts(a)
	if err != nil {
		return 0, err
	}
	right, err := versionParts(b)
	if err != nil {
		return 0, err
	}
	for i := range left {
		if left[i] > right[i] {
			return 1, nil
		}
		if left[i] < right[i] {
			return -1, nil
		}
	}
	return 0, nil
}
