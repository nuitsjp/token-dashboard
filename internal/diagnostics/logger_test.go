package diagnostics

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRotationHasBoundedGenerations(t *testing.T) {
	dir := t.TempDir()
	w := &Writer{path: filepath.Join(dir, "app.jsonl"), limit: 16, generations: 2}
	if err := w.open(); err != nil {
		t.Fatal(err)
	}
	defer w.Close()
	for i := 0; i < 20; i++ {
		if _, err := w.Write([]byte("12345678\n")); err != nil {
			t.Fatal(err)
		}
	}
	files, err := os.ReadDir(dir)
	if err != nil || len(files) != 3 {
		t.Fatalf("rotation: %d %v", len(files), err)
	}
	for _, f := range files {
		info, _ := f.Info()
		if info.Size() > 16 {
			t.Fatal("file exceeded expected limit")
		}
	}
}
