package diagnostics

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
)

type Writer struct {
	mu          sync.Mutex
	path        string
	file        *os.File
	size        int64
	limit       int64
	generations int
	failed      bool
}

func Open(dir string, production bool) (*slog.Logger, io.Closer, error) {
	if !production {
		return slog.New(slog.NewTextHandler(os.Stderr, nil)), nopCloser{}, nil
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, nil, err
	}
	w := &Writer{path: filepath.Join(dir, "app.jsonl"), limit: 2 * 1024 * 1024, generations: 3}
	if err := w.open(); err != nil {
		return nil, nil, err
	}
	return slog.New(slog.NewJSONHandler(w, nil)), w, nil
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }
func (w *Writer) open() error {
	f, err := os.OpenFile(w.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0600)
	if err != nil {
		return err
	}
	info, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	w.file = f
	w.size = info.Size()
	return nil
}
func (w *Writer) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.failed {
		return os.Stderr.Write(p)
	}
	if w.size+int64(len(p)) > w.limit {
		if err := w.rotate(); err != nil {
			w.failed = true
			fmt.Fprintln(os.Stderr, "diagnostic log unavailable:", err)
			return os.Stderr.Write(p)
		}
	}
	n, err := w.file.Write(p)
	w.size += int64(n)
	if err != nil {
		w.failed = true
		fmt.Fprintln(os.Stderr, "diagnostic log unavailable:", err)
		return os.Stderr.Write(p)
	}
	return n, nil
}
func (w *Writer) rotate() error {
	if err := w.file.Close(); err != nil {
		return err
	}
	oldest := fmt.Sprintf("%s.%d", w.path, w.generations)
	if err := os.Remove(oldest); err != nil && !os.IsNotExist(err) {
		return err
	}
	for i := w.generations - 1; i >= 1; i-- {
		if err := os.Rename(fmt.Sprintf("%s.%d", w.path, i), fmt.Sprintf("%s.%d", w.path, i+1)); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	if err := os.Rename(w.path, w.path+".1"); err != nil {
		return err
	}
	return w.open()
}
func (w *Writer) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file != nil {
		return w.file.Close()
	}
	return nil
}
