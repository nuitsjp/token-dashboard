// Package styles stores display styles copied into the app.
package styles

import (
	"crypto/rand"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"token-monitor-turzx/internal/fault"
)

const incomingDir = ".incoming"

type manifest struct {
	FormatVersion int    `json:"formatVersion"`
	ID            string `json:"id"`
	Name          string `json:"name"`
	Width         int    `json:"width"`
	Height        int    `json:"height"`
	Template      string `json:"template"`
	Stylesheet    string `json:"stylesheet"`
}

// ImportDraft is a checked copy that is not yet part of the definitions.
type ImportDraft struct {
	Token string            `json:"token"`
	ID    string            `json:"id"`
	Name  string            `json:"name"`
	Files map[string]string `json:"files"`
}

// StoredStyle is one style already in the app.
type StoredStyle struct {
	ID    string            `json:"id"`
	Name  string            `json:"name"`
	Files map[string]string `json:"files"`
}

type staged struct {
	id  string
	dir string
}

type Service struct {
	dir    string
	logger *slog.Logger
	mu     sync.Mutex
	staged map[string]staged
}

func New(dir string, logger *slog.Logger) *Service {
	return &Service{dir: dir, logger: logger, staged: map[string]staged{}}
}

func invalid() error {
	return fault.New("INVALID", "This folder is not a valid style.")
}
func duplicate() error {
	return fault.New("DUPLICATE", "This style id is already defined.")
}
func (s *Service) copyFailed(err error) error {
	s.logger.Error("style_copy_failed", "cause", err)
	return fault.New("COPY", "Could not copy the style into the app.")
}

// BeginImport checks the folder and copies it aside. FinishImport makes it a definition.
func (s *Service) BeginImport(folder string, definedIDs []string) (draft ImportDraft, err error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanup()
	info, statErr := os.Stat(folder)
	if statErr != nil || !info.IsDir() {
		return ImportDraft{}, invalid()
	}
	manifest, err := readManifest(folder)
	if err != nil {
		return ImportDraft{}, err
	}
	for _, id := range definedIDs {
		if id == manifest.ID {
			return ImportDraft{}, duplicate()
		}
	}
	if s.stored(manifest.ID) {
		return ImportDraft{}, duplicate()
	}
	if !oneDirectoryName(manifest.ID) {
		return ImportDraft{}, s.copyFailed(os.ErrInvalid)
	}
	token, err := newToken()
	if err != nil {
		return ImportDraft{}, s.copyFailed(err)
	}
	dir := filepath.Join(s.dir, incomingDir, token)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return ImportDraft{}, s.copyFailed(err)
	}
	if err := copyTree(folder, dir); err != nil {
		os.RemoveAll(dir)
		return ImportDraft{}, s.copyFailed(err)
	}
	files, err := readTree(dir)
	if err != nil {
		os.RemoveAll(dir)
		return ImportDraft{}, s.copyFailed(err)
	}
	s.staged[token] = staged{id: manifest.ID, dir: dir}
	return ImportDraft{Token: token, ID: manifest.ID, Name: manifest.Name, Files: files}, nil
}

// FinishImport moves a checked copy into the definitions.
func (s *Service) FinishImport(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.staged[token]
	if !ok {
		return s.copyFailed(os.ErrNotExist)
	}
	delete(s.staged, token)
	if s.stored(item.id) {
		os.RemoveAll(item.dir)
		return duplicate()
	}
	dest := filepath.Join(s.dir, item.id)
	if err := os.Rename(item.dir, dest); err != nil {
		os.RemoveAll(item.dir)
		return s.copyFailed(err)
	}
	return nil
}

// CancelImport drops a copy that was not added.
func (s *Service) CancelImport(token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	item, ok := s.staged[token]
	if !ok {
		return nil
	}
	delete(s.staged, token)
	if err := os.RemoveAll(item.dir); err != nil {
		return s.copyFailed(err)
	}
	return nil
}

// List reads the styles kept in the app.
func (s *Service) List() ([]StoredStyle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.cleanup()
	entries, err := os.ReadDir(s.dir)
	if os.IsNotExist(err) {
		return []StoredStyle{}, nil
	}
	if err != nil {
		return nil, s.copyFailed(err)
	}
	list := []StoredStyle{}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == incomingDir {
			continue
		}
		dir := filepath.Join(s.dir, entry.Name())
		manifest, err := readManifest(dir)
		if err != nil || manifest.ID != entry.Name() {
			return nil, invalid()
		}
		files, err := readTree(dir)
		if err != nil {
			return nil, s.copyFailed(err)
		}
		list = append(list, StoredStyle{ID: manifest.ID, Name: manifest.Name, Files: files})
	}
	return list, nil
}

func (s *Service) stored(id string) bool {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() && entry.Name() != incomingDir && strings.EqualFold(entry.Name(), id) {
			return true
		}
	}
	return false
}

func (s *Service) cleanup() {
	root := filepath.Join(s.dir, incomingDir)
	entries, err := os.ReadDir(root)
	if err != nil {
		return
	}
	active := map[string]bool{}
	for token := range s.staged {
		active[token] = true
	}
	for _, entry := range entries {
		if !active[entry.Name()] {
			os.RemoveAll(filepath.Join(root, entry.Name()))
		}
	}
}

func readManifest(dir string) (manifest, error) {
	data, err := os.ReadFile(filepath.Join(dir, "theme.json"))
	if err != nil {
		return manifest{}, invalid()
	}
	var value manifest
	if json.Unmarshal(data, &value) != nil || value.FormatVersion != 1 || value.Width != 1920 || value.Height != 462 || strings.TrimSpace(value.ID) == "" || strings.TrimSpace(value.Name) == "" {
		return manifest{}, invalid()
	}
	if err := directFile(dir, value.Template, ".hbs"); err != nil {
		return manifest{}, err
	}
	if err := directFile(dir, value.Stylesheet, ".css"); err != nil {
		return manifest{}, err
	}
	return value, nil
}

func directFile(dir, value, ext string) error {
	if strings.TrimSpace(value) == "" || filepath.Base(value) != value || filepath.Ext(value) != ext {
		return invalid()
	}
	path := filepath.Join(dir, value)
	info, err := os.Lstat(path)
	if err != nil {
		return invalid()
	}
	if info.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil || !within(dir, resolved) {
			return invalid()
		}
		info, err = os.Stat(resolved)
		if err != nil {
			return invalid()
		}
	}
	if !info.Mode().IsRegular() {
		return invalid()
	}
	return nil
}

func oneDirectoryName(id string) bool {
	if id == "" || id == "." || id == ".." || id == incomingDir {
		return false
	}
	if strings.ContainsAny(id, "<>:\"/\\|?*") || strings.HasSuffix(id, " ") || strings.HasSuffix(id, ".") {
		return false
	}
	for _, r := range id {
		if r < 0x20 {
			return false
		}
	}
	return true
}

func copyTree(src, dst string) error {
	visited := map[string]bool{}
	var walk func(string, string) error
	walk = func(src, dst string) error {
		abs, err := filepath.Abs(src)
		if err != nil {
			return err
		}
		if visited[abs] {
			return nil
		}
		visited[abs] = true
		return copyDir(src, dst, walk)
	}
	return walk(src, dst)
}

func copyDir(src, dst string, walk func(string, string) error) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		if rel == "." {
			return nil
		}
		target := filepath.Join(dst, rel)
		if !within(dst, target) {
			return os.ErrInvalid
		}
		if entry.Type()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(path)
			if err != nil || !within(src, resolved) {
				return nil
			}
			info, err := os.Stat(resolved)
			if err != nil {
				return err
			}
			if info.IsDir() {
				return walk(resolved, target)
			}
			return copyFile(resolved, target)
		}
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyFile(path, target)
	})
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return err
	}
	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()
	if copyErr != nil {
		return copyErr
	}
	return closeErr
}

func readTree(dir string) (map[string]string, error) {
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		files[filepath.ToSlash(rel)] = base64.StdEncoding.EncodeToString(data)
		return nil
	})
	return files, err
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}
	return rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator))
}

func newToken() (string, error) {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
