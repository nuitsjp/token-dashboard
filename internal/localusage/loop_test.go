package localusage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"token-monitor-turzx/internal/usage"
)

// fakeDir names the folder that holds the outputs of the fake tokscale.
const fakeDir = "LOCALUSAGE_FAKE_TOKSCALE"

// TestMain lets the test binary act as tokscale. It takes 300 ms, records when it started and
// ended with its arguments, and prints <first argument>.json from the fake folder, or exits
// with 1 when that file is missing.
func TestMain(m *testing.M) {
	dir := os.Getenv(fakeDir)
	if dir == "" {
		os.Exit(m.Run())
	}
	args := strings.Join(os.Args[1:], " ")
	record := func(event string) {
		log, _ := os.OpenFile(filepath.Join(dir, "calls.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		fmt.Fprintf(log, "%s|%s|%d\n", event, args, time.Now().UnixNano())
		log.Close()
	}
	record("start")
	time.Sleep(300 * time.Millisecond)
	output, err := os.ReadFile(filepath.Join(dir, os.Args[1]+".json"))
	record("end")
	if err != nil {
		os.Exit(1)
	}
	os.Stdout.Write(output)
	os.Exit(0)
}

type span struct{ start, end time.Time }

// runs returns the recorded runs of the fake tokscale with args, in start order.
// Each end closes the earliest open run; the callers check that runs do not overlap.
func runs(t *testing.T, dir, args string) []span {
	t.Helper()
	var out []span
	for _, line := range strings.Split(strings.TrimSpace(readCalls(t, dir)), "\n") {
		parts := strings.Split(line, "|")
		if len(parts) != 3 || parts[1] != args {
			continue
		}
		n, _ := strconv.ParseInt(parts[2], 10, 64)
		at := time.Unix(0, n)
		if parts[0] == "start" {
			out = append(out, span{start: at})
			continue
		}
		for i := range out {
			if out[i].end.IsZero() {
				out[i].end = at
				break
			}
		}
	}
	return out
}

func TestFailedGraphKeepsTheShownUsageAndCursorWithoutAccountIsNotSynced(t *testing.T) {
	dir := t.TempDir()
	logs := filepath.Join(dir, "logs")
	os.Mkdir(logs, 0o700)
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sessions, _ := json.Marshal(logs)
	write("clients.json", `{"clients":[{"client":"claude","sessionsPath":`+string(sessions)+`}]}`)
	write("cursor.json", `{"synced":false,"rows":0,"error":"Not authenticated"}`)
	write("usage.json", `[]`)
	today := time.Now().Format("2006-01-02")
	write("graph.json", `{"contributions":[{"date":"`+today+`","totals":{"cost":1.5},"tokenBreakdown":{"input":1,"output":2,"cacheRead":3,"cacheWrite":4}}]}`)
	t.Setenv(fakeDir, dir)

	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	state := usage.NewState()
	var logged bytes.Buffer
	reader := New(executable, filepath.Join(dir, "config"), filepath.Join(dir, "local-scan.json"), state, slog.New(slog.NewTextHandler(&logged, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		reader.Run(ctx)
		close(done)
	}()
	stop := func() {
		cancel()
		<-done
	}
	defer stop()

	select {
	case <-state.Changed():
	case <-time.After(10 * time.Second):
		t.Fatal("no usage was shown")
	}
	want := usage.Periods{Today: usage.Period{TotalTokens: 10, CostUSD: 1.5}, Month: usage.Period{TotalTokens: 10, CostUSD: 1.5}, AllTime: usage.Period{TotalTokens: 10, CostUSD: 1.5}}
	if got := state.Latest(); got == nil || got.Periods != want {
		t.Fatalf("shown = %+v, want periods %+v", got, want)
	}

	// The next graph fails after a log changes. The shown usage stays as it was.
	os.Remove(filepath.Join(dir, "graph.json"))
	os.WriteFile(filepath.Join(logs, "session.jsonl"), []byte("{}\n"), 0o600)
	deadline := time.Now().Add(20 * time.Second)
	for len(runs(t, dir, "graph --no-spinner")) < 2 {
		if time.Now().After(deadline) {
			t.Fatalf("the change did not run graph again; calls:\n%s", readCalls(t, dir))
		}
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
	if got := state.Latest(); got == nil || got.Periods != want {
		t.Fatalf("after the failure, shown = %+v, want periods %+v", got, want)
	}
	stop()
	if n := len(runs(t, dir, "cursor sync --json")); n != 1 {
		t.Fatalf("cursor sync ran %d times, want once", n)
	}
	if !strings.Contains(logged.String(), "local_usage_graph_failed") {
		t.Fatalf("the failure was not logged:\n%s", logged.String())
	}
}

func readCalls(t *testing.T, dir string) string {
	t.Helper()
	f, err := os.Open(filepath.Join(dir, "calls.log"))
	if err != nil {
		return ""
	}
	defer f.Close()
	b, _ := io.ReadAll(f)
	return string(b)
}

func TestChangesRunGraphAtMostEveryTenSecondsWithoutOverlap(t *testing.T) {
	dir := t.TempDir()
	logs := filepath.Join(dir, "logs")
	os.Mkdir(logs, 0o700)
	sessions, _ := json.Marshal(logs)
	for name, content := range map[string]string{
		"clients.json": `{"clients":[{"client":"claude","sessionsPath":` + string(sessions) + `}]}`,
		"cursor.json":  `{"synced":false,"rows":0,"error":"Not authenticated"}`,
		"usage.json":   `[]`,
		"graph.json":   `{"contributions":[]}`,
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv(fakeDir, dir)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	state := usage.NewState()
	reader := New(executable, filepath.Join(dir, "config"), filepath.Join(dir, "local-scan.json"), state, slog.New(slog.NewTextHandler(io.Discard, nil)))
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		reader.Run(ctx)
		close(done)
	}()
	select {
	case <-state.Changed():
	case <-time.After(10 * time.Second):
		t.Fatal("no usage was shown")
	}
	// Changes keep coming for 12 seconds, more often than the graph may run.
	for i := range 25 {
		os.WriteFile(filepath.Join(logs, "session.jsonl"), []byte(strconv.Itoa(i)), 0o600)
		time.Sleep(500 * time.Millisecond)
	}
	deadline := time.Now().Add(25 * time.Second)
	for len(runs(t, dir, "graph --no-spinner")) < 3 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(time.Second)
	cancel()
	<-done

	graphs := runs(t, dir, "graph --no-spinner")
	if len(graphs) < 3 || len(graphs) > 4 {
		t.Fatalf("graph ran %d times for 12 seconds of changes, want 3 or 4", len(graphs))
	}
	for i := 1; i < len(graphs); i++ {
		if graphs[i].start.Before(graphs[i-1].end) {
			t.Fatalf("graph %d started before graph %d ended", i, i-1)
		}
		// The fake records its start after the process launches, which varies by a few
		// milliseconds, while the reader keeps 10 seconds between the launches.
		if gap := graphs[i].start.Sub(graphs[i-1].start); gap < graphInterval-200*time.Millisecond {
			t.Fatalf("graph %d started %v after the previous one, want at least %v", i, gap, graphInterval)
		}
	}
	limits := runs(t, dir, "usage --json")
	for i := 1; i < len(limits); i++ {
		if limits[i].start.Before(limits[i-1].end) {
			t.Fatalf("usage %d started before usage %d ended", i, i-1)
		}
	}
	// Changes read the limits with each graph, not with each change.
	if len(limits) > len(graphs)+1 {
		t.Fatalf("usage ran %d times with %d graphs", len(limits), len(graphs))
	}
}

func TestSavedScanSkipsClientsUntilANewToolAppears(t *testing.T) {
	dir := t.TempDir()
	logs := filepath.Join(dir, "logs")
	os.Mkdir(logs, 0o700)
	write := func(name, content string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	sessions, _ := json.Marshal(logs)
	write("clients.json", `{"clients":[{"client":"claude","sessionsPath":`+string(sessions)+`},{"client":"cursor","sessionsPath":`+string(sessions)+`}]}`)
	write("cursor.json", `{"synced":false,"rows":0,"error":"Not authenticated"}`)
	write("usage.json", `[]`)
	graphWith := func(clients string) {
		write("graph.json", `{"summary":{"clients":[`+clients+`]},"contributions":[]}`)
	}
	graphWith(`"claude","cursor"`)
	t.Setenv(fakeDir, dir)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	scanFile := filepath.Join(dir, "local-scan.json")
	// start runs a reader until the usage is shown, and returns the function that stops it.
	start := func() func() {
		t.Helper()
		state := usage.NewState()
		reader := New(executable, filepath.Join(dir, "config"), scanFile, state, slog.New(slog.NewTextHandler(io.Discard, nil)))
		ctx, cancel := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			reader.Run(ctx)
			close(done)
		}()
		select {
		case <-state.Changed():
		case <-time.After(10 * time.Second):
			t.Fatal("no usage was shown")
		}
		return func() {
			cancel()
			<-done
		}
	}
	saved := func() scan {
		t.Helper()
		s, err := loadScan(scanFile)
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	scans := func() int { return len(runs(t, dir, "clients --json")) }

	// The first start has no saved scan, so it runs clients and saves the paths without Cursor's.
	stop := start()
	stop()
	if n := scans(); n != 1 {
		t.Fatalf("first start ran clients %d times, want once", n)
	}
	if s := saved(); !reflect.DeepEqual(s.Paths, []string{logs}) || !reflect.DeepEqual(s.Clients, []string{"claude", "cursor"}) {
		t.Fatalf("saved scan = %+v", s)
	}

	// The next start watches the saved paths without running clients.
	stop = start()
	os.WriteFile(filepath.Join(logs, "session.jsonl"), []byte("1"), 0o600)
	deadline := time.Now().Add(20 * time.Second)
	for len(runs(t, dir, "graph --no-spinner")) < 3 && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	if n := len(runs(t, dir, "graph --no-spinner")); n != 3 {
		t.Fatalf("a change under the saved path ran %d graphs in all, want 3", n)
	}
	if n := scans(); n != 1 {
		t.Fatalf("second start ran clients again; %d runs in all", n)
	}

	// A tool that graph shows for the first time runs clients once and is saved.
	graphWith(`"claude","cursor","gemini"`)
	os.WriteFile(filepath.Join(logs, "session.jsonl"), []byte("2"), 0o600)
	deadline = time.Now().Add(30 * time.Second)
	for !slices.Contains(saved().Clients, "gemini") && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	// One more graph with the same tools must not run clients again.
	graphs := len(runs(t, dir, "graph --no-spinner"))
	os.WriteFile(filepath.Join(logs, "session.jsonl"), []byte("3"), 0o600)
	deadline = time.Now().Add(20 * time.Second)
	for len(runs(t, dir, "graph --no-spinner")) == graphs && time.Now().Before(deadline) {
		time.Sleep(100 * time.Millisecond)
	}
	time.Sleep(time.Second)
	stop()
	if len(runs(t, dir, "graph --no-spinner")) == graphs {
		t.Fatal("the last change ran no graph")
	}
	if s := saved(); !reflect.DeepEqual(s.Clients, []string{"claude", "cursor", "gemini"}) {
		t.Fatalf("after the new tool, saved scan = %+v", s)
	}
	if n := scans(); n != 2 {
		t.Fatalf("the new tool ran clients %d times in all, want 2", n)
	}
}
