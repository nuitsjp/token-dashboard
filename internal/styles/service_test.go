package styles

import (
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"token-monitor-turzx/internal/fault"
)

func testService(t *testing.T, dir string) *Service {
	t.Helper()
	return New(dir, slog.New(slog.NewTextHandler(io.Discard, nil)))
}

func writeStyle(t *testing.T, dir, id, name string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(`{"formatVersion":1,"id":"` + id + `","name":"` + name + `","width":1920,"height":462,"template":"template.hbs","stylesheet":"style.css"}`)
	for name, body := range map[string]string{"theme.json": string(manifest), "template.hbs": "<p></p>", "style.css": "body{}"} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func public(t *testing.T, err error) *fault.Error {
	t.Helper()
	var target *fault.Error
	if !errors.As(err, &target) {
		t.Fatalf("error %v", err)
	}
	return target
}

func TestImportKeepsTheCopyWhenTheSourceChanges(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "source")
	writeStyle(t, folder, "night", "Night")
	if err := os.WriteFile(filepath.Join(root, "outside.txt"), []byte("outside"), 0o644); err != nil {
		t.Fatal(err)
	}
	store := filepath.Join(root, "styles")
	service := testService(t, store)
	draft, err := service.BeginImport(folder, nil)
	if err != nil {
		t.Fatal(err)
	}
	if draft.ID != "night" || draft.Name != "Night" {
		t.Fatalf("draft %+v", draft)
	}
	if _, ok := draft.Files["outside.txt"]; ok {
		t.Fatal("copied a file outside the folder")
	}
	if err := service.FinishImport(draft.Token); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "theme.json"), []byte(`{"formatVersion":1,"id":"night","name":"Changed","width":1920,"height":462,"template":"template.hbs","stylesheet":"style.css"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	again := testService(t, store)
	list, err := again.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "night" || list[0].Name != "Night" {
		t.Fatalf("stored %+v", list)
	}
}

func TestImportRejectsInvalidFoldersAndDuplicates(t *testing.T) {
	root := t.TempDir()
	store := filepath.Join(root, "styles")
	service := testService(t, store)
	cases := []struct {
		name string
		body string
	}{
		{"missing", ""},
		{"version", `{"formatVersion":2,"id":"night","name":"Night","width":1920,"height":462,"template":"template.hbs","stylesheet":"style.css"}`},
		{"name", `{"formatVersion":1,"id":"night","name":" ","width":1920,"height":462,"template":"template.hbs","stylesheet":"style.css"}`},
		{"size", `{"formatVersion":1,"id":"night","name":"Night","width":100,"height":462,"template":"template.hbs","stylesheet":"style.css"}`},
		{"template", `{"formatVersion":1,"id":"night","name":"Night","width":1920,"height":462,"template":"nested/template.hbs","stylesheet":"style.css"}`},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			dir := filepath.Join(root, item.name)
			if err := os.MkdirAll(dir, 0o755); err != nil {
				t.Fatal(err)
			}
			if item.body != "" {
				if err := os.WriteFile(filepath.Join(dir, "theme.json"), []byte(item.body), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "template.hbs"), []byte("<p></p>"), 0o644); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(dir, "style.css"), []byte("body{}"), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			_, err := service.BeginImport(dir, nil)
			got := public(t, err)
			if got.Code != "INVALID" || got.Message != "This folder is not a valid style." {
				t.Fatalf("error %+v", got)
			}
		})
	}
	folder := filepath.Join(root, "source")
	writeStyle(t, folder, "night", "Night")
	_, err := service.BeginImport(folder, []string{"night"})
	got := public(t, err)
	if got.Code != "DUPLICATE" || got.Message != "This style id is already defined." {
		t.Fatalf("error %+v", got)
	}
	entries, readErr := os.ReadDir(store)
	if readErr != nil && !os.IsNotExist(readErr) {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("invalid import left %+v", entries)
	}
}

func TestCopyFailureRemovesThePartialCopy(t *testing.T) {
	root := t.TempDir()
	folder := filepath.Join(root, "source")
	writeStyle(t, folder, "night", "Night")
	blocked := filepath.Join(root, "blocked")
	if err := os.WriteFile(blocked, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	service := testService(t, blocked)
	_, err := service.BeginImport(folder, nil)
	got := public(t, err)
	if got.Code != "COPY" || got.Message != "Could not copy the style into the app." {
		t.Fatalf("error %+v", got)
	}
	store := filepath.Join(root, "styles")
	if err := os.MkdirAll(store, 0o755); err != nil {
		t.Fatal(err)
	}
	service = testService(t, store)
	draft, err := service.BeginImport(folder, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := service.CancelImport(draft.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(store, incomingDir, draft.Token)); !os.IsNotExist(err) {
		t.Fatalf("cancelled copy remains: %v", err)
	}
	writeStyle(t, filepath.Join(root, "bad-id"), ".incoming", "Incoming")
	_, err = service.BeginImport(filepath.Join(root, "bad-id"), nil)
	got = public(t, err)
	if got.Code != "COPY" || got.Message != "Could not copy the style into the app." {
		t.Fatalf("error %+v", got)
	}
	draft, err = service.BeginImport(folder, nil)
	if err != nil {
		t.Fatal(err)
	}
	writeStyle(t, filepath.Join(store, "night"), "night", "Already")
	err = service.FinishImport(draft.Token)
	got = public(t, err)
	if got.Code != "DUPLICATE" || got.Message != "This style id is already defined." {
		t.Fatalf("error %+v", got)
	}
	if _, err := os.Stat(filepath.Join(store, incomingDir, draft.Token)); !os.IsNotExist(err) {
		t.Fatalf("rejected copy remains: %v", err)
	}
	list, err := service.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].Name != "Already" {
		t.Fatalf("existing definition changed: %+v", list)
	}
}
