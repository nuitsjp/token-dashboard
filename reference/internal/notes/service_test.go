package notes

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"

	"wailstemplate/internal/appstate"
	"wailstemplate/internal/fault"
)

func testService(t *testing.T, dir string, publish func(string, any)) *Service {
	t.Helper()
	if publish == nil {
		publish = func(string, any) {}
	}
	s, err := New(dir, slog.New(slog.NewTextHandler(io.Discard, nil)), &appstate.State{}, publish)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := s.ServiceShutdown(); err != nil {
			t.Error(err)
		}
	})
	return s
}
func TestSaveAndReload(t *testing.T) {
	dir := t.TempDir()
	s := testService(t, dir, nil)
	ctx := context.Background()
	n, err := s.Save(ctx, SaveRequest{Title: "  first  ", Body: "本文"})
	if err != nil {
		t.Fatal(err)
	}
	if n.ID == "" || n.Title != "first" {
		t.Fatalf("unexpected note %#v", n)
	}
	n2, err := s.Save(ctx, SaveRequest{ID: n.ID, Title: "updated", Body: "変更"})
	if err != nil {
		t.Fatal(err)
	}
	if n2.ID != n.ID {
		t.Fatal("update changed identity")
	}
	restored := testService(t, dir, nil)
	all, err := restored.List(ctx)
	if err != nil || len(all) != 1 || all[0].Body != "変更" {
		t.Fatalf("reload %v %#v", err, all)
	}
	all[0].Title = "mutation outside service"
	actual, _ := restored.Get(ctx, n.ID)
	if actual.Title != "updated" {
		t.Fatal("mutable state escaped")
	}
}
func TestValidationDoesNotSave(t *testing.T) {
	s := testService(t, t.TempDir(), nil)
	_, err := s.Save(context.Background(), SaveRequest{Title: " "})
	var public *fault.Error
	if !errors.As(err, &public) || public.Code != "VALIDATION" {
		t.Fatalf("wrong error %v", err)
	}
	if _, err := os.Stat(filepath.Join(s.dir, "notes.json")); !os.IsNotExist(err) {
		t.Fatal("invalid request wrote a file")
	}
}
func TestCorruptionIsNotReset(t *testing.T) {
	for _, contents := range []string{"invalid", `{"version":2,"notes":[]}`, `{"version":1,"notes":null}`, `{"version":1,"notes":[{"id":"id","title":"title","body":"","updatedAt":"invalid"}]}`} {
		t.Run(contents, func(t *testing.T) {
			dir := t.TempDir()
			path := filepath.Join(dir, "notes.json")
			if err := os.WriteFile(path, []byte(contents), 0600); err != nil {
				t.Fatal(err)
			}
			_, err := New(dir, slog.Default(), &appstate.State{}, func(string, any) {})
			if err == nil {
				t.Fatal("corruption accepted")
			}
			data, _ := os.ReadFile(path)
			if string(data) != contents {
				t.Fatal("corrupted file was overwritten")
			}
		})
	}
}
func TestListSortsByTimestampAndPreservesSavedOrder(t *testing.T) {
	dir := t.TempDir()
	document := document{Version: 1, Notes: []Note{
		{ID: "same-first", Title: "same-first", UpdatedAt: "2025-01-01T00:00:00.5Z"},
		{ID: "offset-equal", Title: "offset-equal", UpdatedAt: "2025-01-01T01:00:00.5+01:00"},
		{ID: "same-second", Title: "same-second", UpdatedAt: "2025-01-01T00:00:00.5Z"},
		{ID: "z-zero", Title: "z-zero", UpdatedAt: "2025-01-01T00:00:00Z"},
		{ID: "z-51", Title: "z-51", UpdatedAt: "2025-01-01T00:00:00.51Z"},
		{ID: "offset-later", Title: "offset-later", UpdatedAt: "2025-01-01T02:00:00+01:00"},
	}}
	data, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "notes.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
	s := testService(t, dir, nil)
	all, err := s.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	got := make([]string, len(all))
	for i, note := range all {
		got[i] = note.ID
	}
	want := []string{"offset-later", "z-51", "same-first", "offset-equal", "same-second", "z-zero"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("unexpected order: got %v, want %v", got, want)
	}
}
func TestCSVPreviewAndAtomicImport(t *testing.T) {
	s := testService(t, t.TempDir(), nil)
	ctx := context.Background()
	text := "title,body\n一つめ,本文\n二つめ,\"改行\nあり\"\n"
	preview, err := s.PreviewImport(ctx, text)
	if err != nil || preview.Count != 2 {
		t.Fatalf("preview %#v %v", preview, err)
	}
	empty, _ := s.List(ctx)
	if len(empty) != 0 {
		t.Fatal("preview mutated data")
	}
	result, err := s.Import(ctx, ImportRequest{CSV: text, OperationID: "request-1"})
	if err != nil || result.Count != 2 {
		t.Fatalf("import %#v %v", result, err)
	}
	progress, err := s.GetImportProgress("request-1")
	if err != nil || progress.Phase != "completed" {
		t.Fatalf("progress %#v %v", progress, err)
	}
	bad := "title,body\nvalid,body\n,invalid\n"
	_, err = s.Import(ctx, ImportRequest{CSV: bad, OperationID: "request-2"})
	if err == nil {
		t.Fatal("invalid batch accepted")
	}
	all, _ := s.List(ctx)
	if len(all) != 2 {
		t.Fatal("partial batch saved")
	}
}
func TestCancellationBeforeCommitKeepsData(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s := testService(t, t.TempDir(), func(name string, data any) {
		if name == ProgressEvent {
			p := data.(ImportProgress)
			if p.Completed == 1 {
				cancel()
			}
		}
	})
	_, err := s.Import(ctx, ImportRequest{CSV: "title,body\na,b\nc,d\n", OperationID: "cancel-test"})
	if err == nil {
		t.Fatal("cancel ignored")
	}
	all, _ := s.List(context.Background())
	if len(all) != 0 {
		t.Fatal("cancelled batch partially committed")
	}
}
func TestWriteFailureDoesNotPublishOrChangeMemory(t *testing.T) {
	count := 0
	s := testService(t, t.TempDir(), func(name string, _ any) {
		if name == ChangedEvent {
			count++
		}
	})
	if err := os.Mkdir(filepath.Join(s.dir, "notes.json"), 0700); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Save(context.Background(), SaveRequest{Title: "no write"}); err == nil {
		t.Fatal("expected storage failure")
	}
	all, _ := s.List(context.Background())
	if len(all) != 0 || count != 0 {
		t.Fatal("failure changed state or published success")
	}
}
func TestConcurrentSavesDoNotLoseData(t *testing.T) {
	s := testService(t, t.TempDir(), nil)
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := s.Save(context.Background(), SaveRequest{Title: "parallel"}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	all, _ := s.List(context.Background())
	if len(all) != 12 {
		t.Fatalf("lost records: %d", len(all))
	}
}
func TestImportLimitsAndHeader(t *testing.T) {
	s := testService(t, t.TempDir(), nil)
	for _, text := range []string{"wrong,header\na,b\n", "title,body\n", strings.Repeat("a", maxImportBytes+1)} {
		if _, err := s.PreviewImport(context.Background(), text); err == nil {
			t.Fatal("invalid input accepted")
		}
	}
}
