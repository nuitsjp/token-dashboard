// Package notes provides the reusable note capability, independent of screens.
package notes

import (
	"context"
	"crypto/rand"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"wailstemplate/internal/appstate"
	"wailstemplate/internal/fault"
)

const ChangedEvent = "notes:changed"
const ProgressEvent = "notes:import-progress"
const maxImportBytes = 1024 * 1024

type Note struct {
	ID        string `json:"id"`
	Title     string `json:"title"`
	Body      string `json:"body"`
	UpdatedAt string `json:"updatedAt"`
}
type SaveRequest struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	Body  string `json:"body"`
}
type ImportRow struct {
	Title string `json:"title"`
	Body  string `json:"body"`
}
type ImportPreview struct {
	Rows  []ImportRow `json:"rows"`
	Count int         `json:"count"`
}
type ImportRequest struct {
	CSV         string `json:"csv"`
	OperationID string `json:"operationID"`
}
type ImportProgress struct {
	OperationID string `json:"operationID"`
	Completed   int    `json:"completed"`
	Total       int    `json:"total"`
	Phase       string `json:"phase"`
}
type ImportResult struct {
	IDs   []string `json:"ids"`
	Count int      `json:"count"`
}
type Changed struct {
	IDs []string `json:"ids"`
}
type document struct {
	Version int    `json:"version"`
	Notes   []Note `json:"notes"`
}

type noteWithTime struct {
	note      Note
	updatedAt time.Time
}

type Service struct {
	mu          sync.RWMutex
	dir         string
	notes       []Note
	logger      *slog.Logger
	state       *appstate.State
	publish     func(string, any)
	operationMu sync.Mutex
	importing   bool
	progress    ImportProgress
	ctx         context.Context
	cancel      context.CancelFunc
	running     sync.WaitGroup
}

// New loads the existing file or creates an empty in-memory collection only for
// a genuinely new profile. Invalid existing files stop startup and are preserved.
func New(dir string, logger *slog.Logger, state *appstate.State, publish func(string, any)) (*Service, error) {
	if logger == nil || state == nil || publish == nil {
		return nil, errors.New("notes: missing dependency")
	}
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(context.Background())
	s := &Service{dir: dir, logger: logger, state: state, publish: publish, notes: []Note{}, ctx: ctx, cancel: cancel}
	b, err := os.ReadFile(filepath.Join(dir, "notes.json"))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		cancel()
		return nil, err
	}
	var d document
	if err = json.Unmarshal(b, &d); err != nil {
		cancel()
		return nil, fmt.Errorf("notes file is invalid: %w", err)
	}
	if d.Version != 1 || d.Notes == nil {
		cancel()
		return nil, errors.New("unsupported notes file format")
	}
	ids := map[string]bool{}
	for _, n := range d.Notes {
		if n.ID == "" || ids[n.ID] {
			cancel()
			return nil, errors.New("invalid or duplicate note ID")
		}
		if err = validate(n.Title, n.Body); err != nil {
			cancel()
			return nil, errors.New("invalid persisted note")
		}
		if _, err = time.Parse(time.RFC3339Nano, n.UpdatedAt); err != nil {
			cancel()
			return nil, errors.New("invalid persisted note")
		}
		ids[n.ID] = true
	}
	s.notes = d.Notes
	return s, nil
}

func (s *Service) List(ctx context.Context) ([]Note, error) {
	if err := ctx.Err(); err != nil {
		return nil, fault.Public(err)
	}
	s.mu.RLock()
	result := append([]Note{}, s.notes...)
	s.mu.RUnlock()
	withTimes := make([]noteWithTime, len(result))
	for i, note := range result {
		// New validates persisted timestamps; Save and Import generate them.
		updatedAt, _ := time.Parse(time.RFC3339Nano, note.UpdatedAt)
		withTimes[i] = noteWithTime{note: note, updatedAt: updatedAt}
	}
	sort.SliceStable(withTimes, func(i, j int) bool {
		return withTimes[i].updatedAt.After(withTimes[j].updatedAt)
	})
	for i, item := range withTimes {
		result[i] = item.note
	}
	return result, nil
}
func (s *Service) Get(ctx context.Context, id string) (Note, error) {
	if err := ctx.Err(); err != nil {
		return Note{}, fault.Public(err)
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	for _, n := range s.notes {
		if n.ID == id {
			return n, nil
		}
	}
	return Note{}, fault.New("NOT_FOUND", "対象のメモが見つかりません。")
}
func (s *Service) Save(ctx context.Context, req SaveRequest) (result Note, err error) {
	defer func() { err = fault.Boundary(s.logger, "notes.save", err) }()
	done, err := s.state.Begin()
	if err != nil {
		return Note{}, err
	}
	defer done()
	if err = ctx.Err(); err != nil {
		return Note{}, err
	}
	req.Title = strings.TrimSpace(req.Title)
	if err = validate(req.Title, req.Body); err != nil {
		return Note{}, err
	}
	s.mu.Lock()
	next := append([]Note{}, s.notes...)
	index := -1
	if req.ID != "" {
		for i, n := range next {
			if n.ID == req.ID {
				index = i
				break
			}
		}
		if index < 0 {
			s.mu.Unlock()
			return Note{}, fault.New("NOT_FOUND", "対象のメモが見つかりません。")
		}
	} else {
		req.ID, err = newID()
		if err != nil {
			s.mu.Unlock()
			return Note{}, err
		}
	}
	result = Note{ID: req.ID, Title: req.Title, Body: req.Body, UpdatedAt: time.Now().UTC().Format(time.RFC3339Nano)}
	if index < 0 {
		next = append(next, result)
	} else {
		next[index] = result
	}
	// Last cancellable point. After replacement succeeds, this is committed.
	if err = ctx.Err(); err == nil {
		err = s.persist(next)
	}
	if err == nil {
		s.notes = next
	}
	s.mu.Unlock()
	if err != nil {
		return Note{}, err
	}
	s.publish(ChangedEvent, Changed{IDs: []string{result.ID}})
	return result, nil
}
func (s *Service) PreviewImport(ctx context.Context, text string) (ImportPreview, error) {
	if err := ctx.Err(); err != nil {
		return ImportPreview{}, fault.Public(err)
	}
	rows, err := parseCSV(text)
	if err != nil {
		return ImportPreview{}, err
	}
	return ImportPreview{Rows: rows, Count: len(rows)}, nil
}
func (s *Service) GetImportProgress(operationID string) (ImportProgress, error) {
	s.operationMu.Lock()
	defer s.operationMu.Unlock()
	if s.progress.OperationID != operationID {
		return ImportProgress{}, fault.New("NOT_FOUND", "対象の取り込み情報がありません。")
	}
	return s.progress, nil
}
func (s *Service) Import(ctx context.Context, req ImportRequest) (result ImportResult, err error) {
	defer func() { err = fault.Boundary(s.logger, "notes.import", err) }()
	if len(req.OperationID) < 1 || len(req.OperationID) > 100 {
		return result, fault.Validation(map[string]string{"operationID": "処理IDが不正です。"})
	}
	done, err := s.state.Begin()
	if err != nil {
		return result, err
	}
	defer done()
	s.operationMu.Lock()
	if s.importing {
		s.operationMu.Unlock()
		return result, fault.New("BUSY", "別の取り込みが実行中です。")
	}
	if s.ctx.Err() != nil {
		s.operationMu.Unlock()
		return result, fault.New("CLOSING", "アプリは終了処理中です。")
	}
	s.importing = true
	s.running.Add(1)
	s.operationMu.Unlock()
	defer func() { s.operationMu.Lock(); s.importing = false; s.operationMu.Unlock(); s.running.Done() }()
	opctx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := context.AfterFunc(s.ctx, cancel)
	defer stop()
	rows, err := parseCSV(req.CSV)
	if err != nil {
		return result, err
	}
	progress := ImportProgress{OperationID: req.OperationID, Total: len(rows), Phase: "validating"}
	report := func(p ImportProgress) {
		s.operationMu.Lock()
		s.progress = p
		s.operationMu.Unlock()
		s.publish(ProgressEvent, p)
	}
	report(progress)
	defer func() {
		if err != nil {
			progress.Phase = "failed"
			if errors.Is(err, context.Canceled) {
				progress.Phase = "cancelled"
			}
			report(progress)
		}
	}()
	now := time.Now().UTC().Format(time.RFC3339Nano)
	additions := make([]Note, 0, len(rows))
	result.IDs = []string{}
	for i, row := range rows {
		if err = opctx.Err(); err != nil {
			return ImportResult{}, err
		}
		var id string
		id, err = newID()
		if err != nil {
			return ImportResult{}, err
		}
		additions = append(additions, Note{ID: id, Title: row.Title, Body: row.Body, UpdatedAt: now})
		result.IDs = append(result.IDs, id)
		progress.Completed = i + 1
		report(progress)
	}
	progress.Phase = "saving"
	report(progress)
	s.mu.Lock()
	next := append(append([]Note{}, s.notes...), additions...)
	if err = opctx.Err(); err == nil {
		err = s.persist(next)
	}
	if err == nil {
		s.notes = next
	}
	s.mu.Unlock()
	if err != nil {
		return ImportResult{}, err
	}
	result.Count = len(additions)
	progress.Phase = "completed"
	report(progress)
	s.publish(ChangedEvent, Changed{IDs: result.IDs})
	return result, nil
}
func (s *Service) ServiceShutdown() error {
	s.operationMu.Lock()
	s.cancel()
	s.operationMu.Unlock()
	done := make(chan struct{})
	go func() { s.running.Wait(); close(done) }()
	select {
	case <-done:
		return nil
	case <-time.After(5 * time.Second):
		return errors.New("notes shutdown timed out")
	}
}
func validate(title, body string) error {
	fields := map[string]string{}
	if strings.TrimSpace(title) == "" || utf8.RuneCountInString(title) > 100 {
		fields["title"] = "タイトルは1〜100文字で入力してください。"
	}
	if utf8.RuneCountInString(body) > 10000 {
		fields["body"] = "本文は10,000文字以内で入力してください。"
	}
	if len(fields) > 0 {
		return fault.Validation(fields)
	}
	return nil
}
func parseCSV(text string) ([]ImportRow, error) {
	if len(text) > maxImportBytes {
		return nil, fault.Validation(map[string]string{"csv": "CSVは1 MiB以下にしてください。"})
	}
	reader := csv.NewReader(strings.NewReader(strings.TrimPrefix(text, "\ufeff")))
	reader.FieldsPerRecord = 2
	header, err := reader.Read()
	if err != nil || len(header) != 2 || header[0] != "title" || header[1] != "body" {
		return nil, fault.Validation(map[string]string{"csv": "先頭行を title,body にしてください。"})
	}
	rows := []ImportRow{}
	for line := 2; ; line++ {
		fields, readErr := reader.Read()
		if errors.Is(readErr, io.EOF) {
			break
		}
		if readErr != nil {
			return nil, fault.Validation(map[string]string{"csv": fmt.Sprintf("%d行目のCSV形式を確認してください。", line)})
		}
		title := strings.TrimSpace(fields[0])
		if validate(title, fields[1]) != nil {
			return nil, fault.Validation(map[string]string{"csv": fmt.Sprintf("%d行目のタイトル・本文の長さを確認してください。", line)})
		}
		rows = append(rows, ImportRow{Title: title, Body: fields[1]})
		if len(rows) > 1000 {
			return nil, fault.Validation(map[string]string{"csv": "参照実装の上限は1,000行です。"})
		}
	}
	if len(rows) == 0 {
		return nil, fault.Validation(map[string]string{"csv": "取り込む行を追加してください。"})
	}
	return rows, nil
}
func newID() (string, error) {
	var data [16]byte
	_, err := rand.Read(data[:])
	return hex.EncodeToString(data[:]), err
}
func (s *Service) persist(notes []Note) error {
	b, err := json.MarshalIndent(document{Version: 1, Notes: notes}, "", "  ")
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(s.dir, ".notes-*.tmp")
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	if _, err = f.Write(b); err == nil {
		err = f.Sync()
	}
	if closeErr := f.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		return err
	}
	return replaceFile(name, filepath.Join(s.dir, "notes.json"))
}
