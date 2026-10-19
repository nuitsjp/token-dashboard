package styles

import (
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"strings"
	"time"

	"token-monitor-turzx/internal/display"
	"token-monitor-turzx/internal/fault"
)

type stamp struct {
	size     int64
	modified time.Time
}

type definition struct {
	manifest manifest
	files    map[string]stamp
}

// Configure wires the immutable built-ins, selection lock, and post-commit notification.
// It is not a bound method and must be called before serving requests.
func Configure(s *Service, builtins map[string]display.Theme, guard func(string, func() error) error, changed func()) {
	s.builtins, s.guardDelete, s.changed = maps.Clone(builtins), guard, changed
}

func unavailable() error {
	return fault.New("STYLE_UNAVAILABLE", "The selected style is unavailable. USB output is paused.")
}

// Catalog snapshots committed metadata. Only the selected external copy is stat-checked;
// no image, template, or stylesheet bytes are reread on the two-second configuration poll.
func Catalog(s *Service, selected string) (map[string]display.Theme, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.loadDefinitions(); err != nil {
		return nil, err
	}
	if selected != "" {
		if _, builtin := s.builtins[selected]; !builtin {
			item, exists := s.definitions[selected]
			if !exists || item.check(filepath.Join(s.dir, selected)) != nil {
				return nil, unavailable()
			}
		}
	}
	themes := maps.Clone(s.builtins)
	if themes == nil {
		themes = map[string]display.Theme{}
	}
	for id, item := range s.definitions {
		m := item.manifest
		themes[id] = display.Theme{ID: id, Name: m.Name, Width: m.Width, Height: m.Height, DataLayout: m.DataLayout}
	}
	return themes, nil
}

func (s *Service) loadDefinitions() error {
	if s.definitions != nil {
		return nil
	}
	entries, err := os.ReadDir(s.dir)
	if os.IsNotExist(err) {
		s.definitions = map[string]definition{}
		return nil
	}
	if err != nil {
		return s.copyFailed(err)
	}
	next := map[string]definition{}
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == incomingDir {
			continue
		}
		dir := filepath.Join(s.dir, entry.Name())
		m, err := readManifest(dir)
		if err != nil || m.ID != entry.Name() || !oneDirectoryName(m.ID) {
			return invalid()
		}
		for id := range s.builtins {
			if strings.EqualFold(id, m.ID) {
				return duplicate()
			}
		}
		item, err := inspectDefinition(dir, m)
		if err != nil {
			return invalid()
		}
		next[m.ID] = item
	}
	s.definitions = next
	return nil
}

func inspectDefinition(dir string, m manifest) (definition, error) {
	item := definition{manifest: m, files: map[string]stamp{}}
	err := filepath.WalkDir(dir, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return os.ErrInvalid
		}
		relative, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		item.files[relative] = stamp{size: info.Size(), modified: info.ModTime()}
		return nil
	})
	return item, err
}

func (d definition) check(dir string) error {
	for relative, before := range d.files {
		info, err := os.Stat(filepath.Join(dir, relative))
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() || before.size != info.Size() || !before.modified.Equal(info.ModTime()) {
			return os.ErrInvalid
		}
	}
	return nil
}
