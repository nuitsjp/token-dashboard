package styles

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"token-monitor-turzx/internal/display"
)

func TestImportedManifestSizeAndLayoutContract(t *testing.T) {
	for _, tc := range []struct {
		name          string
		width, height int
		layout        any
		omit, valid   bool
		want          string
	}{
		{"legacy", 1920, 462, nil, true, true, "gauges"},
		{"large gauges", 1920, 462, "gauges", false, true, "gauges"},
		{"large bars", 1920, 462, "bars", false, true, "bars"},
		{"landscape gauges", 480, 320, "service-gauges", false, true, "service-gauges"},
		{"landscape bars", 480, 320, "service-bars", false, true, "service-bars"},
		{"portrait gauges", 320, 480, "service-gauges", false, true, "service-gauges"},
		{"portrait bars", 320, 480, "service-bars", false, true, "service-bars"},
		{"small omitted", 480, 320, nil, true, false, ""},
		{"large empty", 1920, 462, "", false, false, ""},
		{"large null", 1920, 462, nil, false, false, ""},
		{"small large layout", 320, 480, "bars", false, false, ""},
		{"large service layout", 1920, 462, "service-gauges", false, false, ""},
		{"unknown layout", 480, 320, "custom", false, false, ""},
		{"unknown dimensions", 480, 480, "service-bars", false, false, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			folder, store := filepath.Join(root, "source"), filepath.Join(root, "styles")
			writeStyle(t, folder, "external", "External")
			m := map[string]any{"formatVersion": 1, "id": "external", "name": "External", "width": tc.width, "height": tc.height, "template": "template.hbs", "stylesheet": "style.css"}
			if !tc.omit {
				m["dataLayout"] = tc.layout
			}
			data, _ := json.Marshal(m)
			if err := os.WriteFile(filepath.Join(folder, "theme.json"), data, 0o600); err != nil {
				t.Fatal(err)
			}
			s := testService(t, store)
			Configure(s, map[string]display.Theme{"gauges": {ID: "gauges"}}, nil, nil)
			draft, err := s.BeginImport(folder, nil)
			if !tc.valid {
				if public(t, err).Code != "INVALID" {
					t.Fatalf("expected invalid style: %v", err)
				}
				return
			}
			if err != nil || draft.Width != tc.width || draft.Height != tc.height || draft.DataLayout != tc.want {
				t.Fatalf("draft metadata: %+v, %v", draft, err)
			}
			before, err := Catalog(s, "gauges")
			if err != nil || len(before) != 1 {
				t.Fatalf("uncommitted import was published: %v, %v", before, err)
			}
			if err := s.FinishImport(draft.Token); err != nil {
				t.Fatal(err)
			}
			// A new service exercises startup restoration from the internal copy.
			restarted := testService(t, store)
			Configure(restarted, before, nil, nil)
			catalog, err := Catalog(restarted, "external")
			if err != nil || catalog["external"].Width != tc.width || catalog["external"].Height != tc.height || catalog["external"].DataLayout != tc.want {
				t.Fatalf("restored metadata: %+v, %v", catalog, err)
			}
		})
	}
}

func TestImportRejectsBuiltinIDWithoutTrustingCallerIDs(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "source")
	writeStyle(t, folder, "GAUGES", "External")
	s := testService(t, filepath.Join(root, "styles"))
	Configure(s, map[string]display.Theme{"gauges": {ID: "gauges"}}, nil, nil)
	if public(t, func() error { _, err := s.BeginImport(folder, nil); return err }()).Code != "DUPLICATE" {
		t.Fatal("a caller omitting built-in IDs could overwrite a built-in")
	}
}

func TestChangedStagedFilesNeverBecomeSelectable(t *testing.T) {
	root := t.TempDir()
	folder, store := filepath.Join(root, "source"), filepath.Join(root, "styles")
	writeStyle(t, folder, "external", "External")
	s := testService(t, store)
	notifications := 0
	Configure(s, nil, nil, func() { notifications++ })
	draft, err := s.BeginImport(folder, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store, incomingDir, draft.Token, "style.css"), []byte("changed after validation"), 0o600); err != nil {
		t.Fatal(err)
	}
	if public(t, s.FinishImport(draft.Token)).Code != "INVALID" {
		t.Fatal("changed staged files were committed")
	}
	catalog, err := Catalog(s, "")
	if err != nil || len(catalog) != 0 || notifications != 0 {
		t.Fatalf("failed import published metadata or notification: %v, %v", catalog, err)
	}
	if entries, err := os.ReadDir(filepath.Join(store, incomingDir)); err != nil || len(entries) != 0 {
		t.Fatalf("failed import left temporary files: %v, %v", entries, err)
	}
}

func TestReadIsolatesUnavailableStoredStyle(t *testing.T) {
	root := t.TempDir()
	store := filepath.Join(root, "styles")
	service := testService(t, store)
	for _, item := range []struct {
		id   string
		name string
	}{
		{id: "healthy", name: "Healthy"},
		{id: "broken", name: "Broken"},
	} {
		source := filepath.Join(root, item.id)
		writeStyle(t, source, item.id, item.name)
		if err := os.WriteFile(filepath.Join(source, "asset.svg"), []byte(item.id), 0o644); err != nil {
			t.Fatal(err)
		}
		draft, err := service.BeginImport(source, nil)
		if err != nil {
			t.Fatal(err)
		}
		if err := service.FinishImport(draft.Token); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Remove(filepath.Join(store, "broken", "asset.svg")); err != nil {
		t.Fatal(err)
	}

	if _, err := service.List(); public(t, err).Code != "STYLE_UNAVAILABLE" {
		t.Fatal("listing should fail when one stored style is unavailable")
	}
	got, err := service.Read("healthy")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "healthy" || got.Name != "Healthy" || got.Width != 1920 || got.Height != 462 || got.DataLayout != "gauges" {
		t.Fatalf("read metadata: %+v", got)
	}
	if got.Files["theme.json"] == "" || got.Files["template.hbs"] == "" || got.Files["style.css"] == "" || got.Files["asset.svg"] == "" || len(got.Files) != 4 {
		t.Fatalf("read files: %+v", got.Files)
	}
	if public(t, func() error { _, err := service.Read("broken"); return err }()).Code != "STYLE_UNAVAILABLE" {
		t.Fatal("reading the unavailable style should fail")
	}
	for _, id := range []string{"missing", "../healthy"} {
		if public(t, func() error { _, err := service.Read(id); return err }()).Code != "STYLE_UNAVAILABLE" {
			t.Fatalf("reading %q should fail", id)
		}
	}

	restarted := testService(t, store)
	if got, err := restarted.Read("healthy"); err != nil || got.ID != "healthy" || got.Name != "Healthy" || len(got.Files) != 4 {
		t.Fatalf("read after restart: %+v, %v", got, err)
	}
}
