package localusage

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"token-monitor-turzx/internal/usage"
)

// testIntervals are short so that the tests do not wait for real time.
var testIntervals = Intervals{Settle: 50 * time.Millisecond, Graph: 500 * time.Millisecond, Poll: 2 * time.Second, MaxSyncDelay: 4 * time.Second}

// TestMain lets the test binary act as tokscale when the reader runs it with a TOKSCALE_CONFIG_DIR
// whose parent holds a fake-tokscale file. Each test has its own folder, so tests run in parallel.
func TestMain(m *testing.M) {
	if os.Getenv(holdOutput) != "" {
		// A process that tokscale started, which keeps tokscale's output open.
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	if config := os.Getenv("TOKSCALE_CONFIG_DIR"); config != "" {
		dir := filepath.Dir(config)
		if _, err := os.Stat(filepath.Join(dir, "fake-tokscale")); err == nil {
			actAsTokscale(dir)
		}
	}
	os.Exit(m.Run())
}

// holdOutput makes the test binary a process that only keeps its output open for 30 seconds.
const holdOutput = "LOCALUSAGE_HOLD_OUTPUT"

// actAsTokscale takes 50 ms, records when it started and ended with its arguments, and prints
// <first argument>.json from dir, or exits with 1 when that file is missing. When
// <first argument>.hold exists, it instead starts a process that shares its output, and both
// wait for 30 seconds.
func actAsTokscale(dir string) {
	args := strings.Join(os.Args[1:], " ")
	record := func(event string) {
		log, _ := os.OpenFile(filepath.Join(dir, "calls.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o600)
		fmt.Fprintf(log, "%s|%s|%d\n", event, args, time.Now().UnixNano())
		log.Close()
	}
	record("start")
	if _, err := os.Stat(filepath.Join(dir, os.Args[1]+".hold")); err == nil {
		child := exec.Command(os.Args[0])
		child.Env = append(os.Environ(), holdOutput+"=1")
		child.Stdout = os.Stdout
		if child.Start() == nil {
			os.WriteFile(filepath.Join(dir, "held.pid"), []byte(strconv.Itoa(child.Process.Pid)), 0o600)
		}
		time.Sleep(30 * time.Second)
		os.Exit(0)
	}
	time.Sleep(50 * time.Millisecond)
	output, err := os.ReadFile(filepath.Join(dir, os.Args[1]+".json"))
	record("end")
	if err != nil {
		os.Exit(1)
	}
	os.Stdout.Write(output)
	os.Exit(0)
}

// fakeTokscale is the folder of one test: the outputs of the fake tokscale, its record of runs,
// and a logs folder that clients.json returns as Claude Code's scan location.
type fakeTokscale struct {
	t    *testing.T
	dir  string
	logs string
}

func newFakeTokscale(t *testing.T) *fakeTokscale {
	t.Helper()
	dir := t.TempDir()
	f := &fakeTokscale{t: t, dir: dir, logs: filepath.Join(dir, "logs")}
	if err := os.Mkdir(f.logs, 0o700); err != nil {
		t.Fatal(err)
	}
	sessions, _ := json.Marshal(f.logs)
	f.write("fake-tokscale", "")
	f.write("clients.json", `{"clients":[{"client":"claude","sessionsPath":`+string(sessions)+`},{"client":"cursor","sessionsPath":`+string(sessions)+`}]}`)
	f.write("cursor.json", `{"synced":false,"rows":0,"error":"Not authenticated"}`)
	f.write("usage.json", `[]`)
	f.write("graph.json", `{"summary":{"clients":["claude"]},"contributions":[]}`)
	return f
}

func (f *fakeTokscale) write(name, content string) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(f.dir, name), []byte(content), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

// change writes a log under the watched scan location.
func (f *fakeTokscale) change(content string) {
	f.t.Helper()
	if err := os.WriteFile(filepath.Join(f.logs, "session.jsonl"), []byte(content), 0o600); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fakeTokscale) scanFile() string { return filepath.Join(f.dir, "local-scan.json") }

// start runs a reader until it shows usage, and returns the state and the function that stops it.
func (f *fakeTokscale) start(logger *slog.Logger) (*usage.State, func()) {
	f.t.Helper()
	executable, err := os.Executable()
	if err != nil {
		f.t.Fatal(err)
	}
	state := usage.NewState()
	reader := New(executable, filepath.Join(f.dir, "config"), f.scanFile(), testIntervals, state, logger)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		reader.Run(ctx)
		close(done)
	}()
	stopped := false
	stop := func() {
		if !stopped {
			stopped = true
			cancel()
			<-done
		}
	}
	f.t.Cleanup(stop)
	select {
	case <-state.Changed():
	case <-time.After(10 * time.Second):
		f.t.Fatal("no usage was shown")
	}
	return state, stop
}

type span struct{ start, end time.Time }

// runs returns the recorded runs with args, in start order. Each end closes the earliest open
// run; the callers check that runs do not overlap.
func (f *fakeTokscale) runs(args string) []span {
	data, _ := os.ReadFile(filepath.Join(f.dir, "calls.log"))
	var out []span
	for _, line := range strings.Split(strings.TrimSpace(string(data)), "\n") {
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

// waitFor polls cond until it holds, or fails the test after timeout.
func waitFor(t *testing.T, timeout time.Duration, what string, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

func quiet() *slog.Logger { return slog.New(slog.NewTextHandler(io.Discard, nil)) }

func TestFailedGraphKeepsTheShownUsageAndCursorWithoutAccountIsNotSynced(t *testing.T) {
	t.Parallel()
	f := newFakeTokscale(t)
	today := time.Now().Format("2006-01-02")
	f.write("graph.json", `{"summary":{"clients":["claude"]},"contributions":[{"date":"`+today+`","totals":{"cost":1.5},"tokenBreakdown":{"input":1,"output":2,"cacheRead":3,"cacheWrite":4}}]}`)
	var logged bytes.Buffer
	state, stop := f.start(slog.New(slog.NewTextHandler(&logged, nil)))
	want := usage.Periods{Today: usage.Period{TotalTokens: 10, CostUSD: 1.5}, Month: usage.Period{TotalTokens: 10, CostUSD: 1.5}, AllTime: usage.Period{TotalTokens: 10, CostUSD: 1.5}}
	if got := state.Latest(); got == nil || got.Periods != want {
		t.Fatalf("shown = %+v, want periods %+v", got, want)
	}

	// The next graph fails after a log changes. The shown usage stays as it was.
	os.Remove(filepath.Join(f.dir, "graph.json"))
	f.change("1")
	waitFor(t, 15*time.Second, "the graph after the change", func() bool { return len(f.runs("graph --no-spinner")) >= 2 })
	// Longer than one poll, so that a Cursor sync would have run again if it were on.
	time.Sleep(testIntervals.Poll + 500*time.Millisecond)
	stop()
	if got := state.Latest(); got == nil || got.Periods != want {
		t.Fatalf("after the failure, shown = %+v, want periods %+v", got, want)
	}
	if n := len(f.runs("cursor sync --json")); n != 1 {
		t.Fatalf("cursor sync ran %d times, want once", n)
	}
	if !strings.Contains(logged.String(), "local_usage_graph_failed") {
		t.Fatalf("the failure was not logged:\n%s", logged.String())
	}
}

func TestChangesRunGraphAtMostOncePerIntervalWithoutOverlap(t *testing.T) {
	t.Parallel()
	f := newFakeTokscale(t)
	_, stop := f.start(quiet())
	// Changes keep coming for 1.5 seconds, more often than the graph may run.
	for i := range 30 {
		f.change(strconv.Itoa(i))
		time.Sleep(50 * time.Millisecond)
	}
	waitFor(t, 15*time.Second, "three graphs", func() bool { return len(f.runs("graph --no-spinner")) >= 3 })
	time.Sleep(testIntervals.Graph)
	stop()

	graphs := f.runs("graph --no-spinner")
	for i := 1; i < len(graphs); i++ {
		if graphs[i].start.Before(graphs[i-1].end) {
			t.Fatalf("graph %d started before graph %d ended", i, i-1)
		}
		// The fake records its start after the process launches, which can vary by tens of
		// milliseconds on a busy machine, while the reader keeps the interval between the launches.
		// Without the interval, graphs would follow each change about every 50 milliseconds.
		if gap := graphs[i].start.Sub(graphs[i-1].start); gap < testIntervals.Graph*7/10 {
			t.Fatalf("graph %d started %v after the previous one, want at least %v", i, gap, testIntervals.Graph)
		}
	}
	limits := f.runs("usage --json")
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

func TestLimitsAreReadAgainEveryPollWithoutChanges(t *testing.T) {
	t.Parallel()
	f := newFakeTokscale(t)
	_, stop := f.start(quiet())
	time.Sleep(2*testIntervals.Poll + testIntervals.Poll/2)
	stop()
	limits := f.runs("usage --json")
	if len(limits) < 3 {
		t.Fatalf("usage ran %d times in 2.5 polls after the start, want at least 3", len(limits))
	}
	for i := 1; i < len(limits); i++ {
		// The timer counts from the request, the record from after the launch, which takes up to a few hundred milliseconds when other runs start at once.
		if gap := limits[i].start.Sub(limits[i-1].start); gap < testIntervals.Poll-500*time.Millisecond || gap > testIntervals.Poll+500*time.Millisecond {
			t.Fatalf("usage %d started %v after the previous one, want about %v", i, gap, testIntervals.Poll)
		}
	}
	if n := len(f.runs("graph --no-spinner")); n != 1 {
		t.Fatalf("graph ran %d times without changes, want once", n)
	}
}

func TestSavedScanSkipsClientsUntilANewToolAppears(t *testing.T) {
	t.Parallel()
	f := newFakeTokscale(t)
	f.write("graph.json", `{"summary":{"clients":["claude","cursor"]},"contributions":[]}`)
	saved := func() scan {
		t.Helper()
		s, err := loadScan(f.scanFile())
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	scans := func() int { return len(f.runs("clients --json")) }

	// The first start has no saved scan, so it runs clients and saves the paths without Cursor's.
	_, stop := f.start(quiet())
	stop()
	if n := scans(); n != 1 {
		t.Fatalf("first start ran clients %d times, want once", n)
	}
	if s := saved(); !reflect.DeepEqual(s.Paths, []string{f.logs}) || !reflect.DeepEqual(s.Clients, []string{"claude", "cursor"}) {
		t.Fatalf("saved scan = %+v", s)
	}

	// The next start watches the saved paths without running clients.
	_, stop = f.start(quiet())
	f.change("1")
	waitFor(t, 15*time.Second, "the graph after the change", func() bool { return len(f.runs("graph --no-spinner")) >= 3 })
	if n := scans(); n != 1 {
		t.Fatalf("second start ran clients again; %d runs in all", n)
	}

	// A tool that graph shows for the first time runs clients once and is saved.
	f.write("graph.json", `{"summary":{"clients":["claude","cursor","gemini"]},"contributions":[]}`)
	f.change("2")
	// The file is read only after the reader stops: while it is open, Windows refuses to replace it.
	finishedAfter := func(at time.Time) bool {
		for _, g := range f.runs("graph --no-spinner") {
			if g.start.After(at) && !g.end.IsZero() {
				return true
			}
		}
		return false
	}
	waitFor(t, 15*time.Second, "clients to run again", func() bool { return scans() == 2 })
	waitFor(t, 15*time.Second, "the graph that saves the new tool", func() bool {
		s := f.runs("clients --json")
		return !s[1].end.IsZero() && finishedAfter(s[1].end)
	})
	// One more graph with the same tools must not run clients again.
	changed := time.Now()
	f.change("3")
	waitFor(t, 15*time.Second, "the graph after the last change", func() bool { return finishedAfter(changed) })
	stop()
	if n := scans(); n != 2 {
		t.Fatalf("the new tool ran clients %d times in all, want 2", n)
	}
	if s := saved(); !slices.Contains(s.Clients, "gemini") {
		t.Fatalf("after the new tool, saved scan = %+v", s)
	}
}

func TestStopDoesNotWaitForProcessesThatTokscaleStarted(t *testing.T) {
	t.Parallel()
	f := newFakeTokscale(t)
	_, stop := f.start(quiet())
	// The next periodic usage read starts a process that keeps the output open. It is ended
	// after the test, since it runs from the test binary that go test removes.
	f.write("usage.hold", "")
	t.Cleanup(func() {
		data, _ := os.ReadFile(filepath.Join(f.dir, "held.pid"))
		pid, _ := strconv.Atoi(string(data))
		if p, err := os.FindProcess(pid); err == nil && pid != 0 {
			p.Kill()
			p.Wait()
		}
	})
	waitFor(t, 15*time.Second, "the held usage read", func() bool {
		_, err := os.Stat(filepath.Join(f.dir, "held.pid"))
		return err == nil
	})
	started := time.Now()
	stop()
	if took := time.Since(started); took > 3*time.Second {
		t.Fatalf("stopping took %v, want it not to wait for the process tokscale started", took)
	}
}
