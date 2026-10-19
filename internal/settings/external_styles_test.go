//go:build windows

package settings

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"token-monitor-turzx/internal/display"
	"token-monitor-turzx/internal/fault"
	"token-monitor-turzx/internal/styles"
	"token-monitor-turzx/internal/turzx"
)

func externalStyles(t *testing.T) (*Service, *styles.Service, string, string) {
	t.Helper()
	devices := []turzx.Device{{ID: smallDisplay}}
	s, path := newService(t, &devices)
	ConfigureDisplay(s, presentationThemes())
	store := filepath.Join(t.TempDir(), "styles")
	added := styles.New(store, slog.New(slog.NewTextHandler(io.Discard, nil)))
	styles.Configure(added, presentationThemes(), func(id string, remove func() error) error {
		return DeleteStyle(s, id, remove)
	}, nil)
	ConfigureCatalog(s, func(selected string) (map[string]display.Theme, error) { return styles.Catalog(added, selected) })
	folder := t.TempDir()
	for name, data := range map[string]string{
		"theme.json":   `{"formatVersion":1,"id":"external","name":"External","width":320,"height":480,"dataLayout":"service-bars","template":"template.hbs","stylesheet":"style.css"}`,
		"template.hbs": "<main></main>", "style.css": "body{}",
	} {
		if err := os.WriteFile(filepath.Join(folder, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	draft, err := added.BeginImport(folder, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := added.FinishImport(draft.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(SaveRequest{Source: "Local", DisplayID: smallDisplay, LimitStyle: "gauges-35-landscape", ServiceCycleSeconds: 19}); err != nil {
		t.Fatal(err)
	}
	return s, added, path, store
}

func errorCode(err error) string {
	var value *fault.Error
	if errors.As(err, &value) {
		return value.Code
	}
	return ""
}

func TestExternalCatalogChangesLeaveGenerationAndSelectionUnchanged(t *testing.T) {
	s, added, path, store := externalStyles(t)
	before, _ := s.Get()
	data, _ := os.ReadFile(path)
	notifications := 0
	styles.Configure(added, presentationThemes(), func(id string, remove func() error) error {
		return DeleteStyle(s, id, remove)
	}, func() {
		// Reading configuration in the notification also checks that both locks were released.
		if _, err := s.Get(); err != nil {
			t.Error(err)
		}
		notifications++
	})
	folder := t.TempDir()
	for _, name := range []string{"theme.json", "template.hbs", "style.css"} {
		content, err := os.ReadFile(filepath.Join(store, "external", name))
		if err != nil {
			t.Fatal(err)
		}
		if name == "theme.json" {
			content = bytes.ReplaceAll(content, []byte(`"external"`), []byte(`"second"`))
		}
		if err := os.WriteFile(filepath.Join(folder, name), content, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	draft, err := added.BeginImport(folder, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := added.FinishImport(draft.Token); err != nil {
		t.Fatal(err)
	}
	view, err := s.Get()
	if err != nil || len(view.Themes) != len(before.Themes)+1 {
		t.Fatalf("committed definition not published: %+v, %v", view, err)
	}
	if err := added.Delete("second"); err != nil {
		t.Fatal(err)
	}
	after, err := s.Get()
	current, _ := os.ReadFile(path)
	if err != nil || notifications != 2 || after.Generation != before.Generation || after.LimitStyle != before.LimitStyle || !bytes.Equal(current, data) {
		t.Fatalf("catalog operation changed presentation state: notifications=%d err=%v", notifications, err)
	}
}

func TestExternalSelectionAndDeletionAreSerialized(t *testing.T) {
	for range 20 {
		s, added, path, store := externalStyles(t)
		start := make(chan struct{})
		var saved, deleted error
		var workers sync.WaitGroup
		workers.Go(func() {
			<-start
			_, saved = s.Save(SaveRequest{Source: "Local", DisplayID: smallDisplay, LimitStyle: "external"})
		})
		workers.Go(func() { <-start; deleted = added.Delete("external") })
		close(start)
		workers.Wait()
		view, err := s.Get()
		if err != nil {
			t.Fatal(err)
		}
		_, statErr := os.Stat(filepath.Join(store, "external"))
		if saved == nil {
			if errorCode(deleted) != "IN_USE" || view.LimitStyle != "external" || statErr != nil {
				t.Fatalf("selected copy was removed: delete=%v style=%s stat=%v", deleted, view.LimitStyle, statErr)
			}
		} else if deleted != nil || errorCode(saved) != "VALIDATION" || view.LimitStyle != "gauges-35-landscape" || !os.IsNotExist(statErr) {
			t.Fatalf("deleted copy became selected: save=%v delete=%v style=%s stat=%v", saved, deleted, view.LimitStyle, statErr)
		}
		if view.ServiceCycleSeconds != 19 {
			t.Fatalf("unrelated interval changed in %s", path)
		}
	}
}

func TestUnavailableSelectedExternalStylePreservesSettingsAndCanRecover(t *testing.T) {
	for _, failure := range []string{"missing asset", "changed asset", "restart missing copy", "restart corrupt manifest"} {
		t.Run(failure, func(t *testing.T) {
			s, _, path, store := externalStyles(t)
			before, err := s.Save(SaveRequest{Source: "Local", DisplayID: smallDisplay, LimitStyle: "external"})
			if err != nil {
				t.Fatal(err)
			}
			data, _ := os.ReadFile(path)
			css := filepath.Join(store, "external", "style.css")
			switch failure {
			case "missing asset":
				err = os.Rename(css, filepath.Join(store, "held.css"))
			case "changed asset":
				err = os.WriteFile(css, []byte("corrupted internal copy"), 0o600)
			case "restart missing copy":
				err = os.RemoveAll(filepath.Join(store, "external"))
			case "restart corrupt manifest":
				err = os.WriteFile(filepath.Join(store, "external", "theme.json"), []byte("{"), 0o600)
			}
			if err != nil {
				t.Fatal(err)
			}
			if failure == "restart missing copy" || failure == "restart corrupt manifest" {
				added := styles.New(store, slog.New(slog.NewTextHandler(io.Discard, nil)))
				styles.Configure(added, presentationThemes(), nil, nil)
				ConfigureCatalog(s, func(selected string) (map[string]display.Theme, error) { return styles.Catalog(added, selected) })
			}
			for range 3 {
				if _, err := s.Get(); err == nil {
					t.Fatal("unavailable selection was silently replaced")
				}
				if _, err := DisplayConfiguration(s); err == nil {
					t.Fatal("unavailable style was allowed to render")
				}
			}
			current, _ := os.ReadFile(path)
			if !bytes.Equal(current, data) || s.generation != before.Generation {
				t.Fatal("style error changed settings or rendering generation")
			}
			if failure == "missing asset" {
				if err := os.Rename(filepath.Join(store, "held.css"), css); err != nil {
					t.Fatal(err)
				}
				if config, err := DisplayConfiguration(s); err != nil || config.Theme.ID != "external" || config.Theme.Width != 320 {
					t.Fatalf("restored asset did not recover: %+v, %v", config, err)
				}
			}
		})
	}
}

func TestExternalSelectionSaveFailureDoesNotAdvanceGeneration(t *testing.T) {
	s, added, path, _ := externalStyles(t)
	before, _ := s.Get()
	data, _ := os.ReadFile(path)
	if err := os.Chmod(path, 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o600) })
	if _, err := s.Save(SaveRequest{Source: "Local", DisplayID: smallDisplay, LimitStyle: "external"}); err == nil {
		t.Fatal("read-only settings were replaced")
	}
	current, _ := os.ReadFile(path)
	after, _ := s.Get()
	if !bytes.Equal(data, current) || before.Generation != after.Generation || after.LimitStyle != before.LimitStyle {
		t.Fatal("failed selection changed settings or generation")
	}
	// The failed selection must not lock the unselected definition against deletion.
	if err := added.Delete("external"); err != nil {
		t.Fatal(err)
	}
	if s.generation != before.Generation {
		t.Fatal("unselected deletion restarted rendering")
	}
}
