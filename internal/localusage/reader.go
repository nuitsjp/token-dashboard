// Package localusage reads local token usage from tokscale.
package localusage

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"os"
	"os/exec"
	"reflect"
	"sync"
	"time"

	"token-monitor-turzx/internal/usage"
)

const (
	// settleDelay gathers the changes that follow the first one into one update.
	settleDelay = 2 * time.Second
	// graphInterval is the least time between the starts of two graph runs.
	graphInterval = 10 * time.Second
	// pollInterval is the period of usage limits and Cursor syncs.
	pollInterval = 45 * time.Second
	// maxSyncDelay caps the growing wait after Cursor sync failures.
	maxSyncDelay = 10 * time.Minute
)

// Reader keeps the state current with usage from a local tokscale executable.
type Reader struct {
	executable string
	configDir  string
	scanFile   string
	state      *usage.State
	logger     *slog.Logger
}

// New creates a local tokscale reader. configDir holds the tokscale settings and caches,
// and scanFile the scan locations to watch.
func New(executable, configDir, scanFile string, state *usage.State, logger *slog.Logger) *Reader {
	return &Reader{executable: executable, configDir: configDir, scanFile: scanFile, state: state, logger: logger}
}

type job int

const (
	graphJob job = iota
	usageJob
	syncJob
	scanJob
)

type result struct {
	job     job
	periods usage.Periods
	clients []string
	limits  usage.Limits
	cursor  cursorOutcome
	paths   []string
	err     error
}

// afterScan is what the next successful graph does with its clients after `clients --json`.
type afterScan int

const (
	scanDone afterScan = iota
	// scanSave saves the new paths with the graph's clients.
	scanSave
	// scanAdopt takes the graph's clients as known without saving, after a failed scan,
	// so that the same tools do not start another scan.
	scanAdopt
)

// loop is the state of one Run. Only the Run goroutine touches it.
type loop struct {
	r       *Reader
	ctx     context.Context
	wg      sync.WaitGroup
	results chan result

	periods   *usage.Periods
	limits    *usage.Limits
	published *usage.Stats

	changed   chan struct{}
	stopWatch context.CancelFunc
	paths     []string
	known     map[string]bool
	afterScan afterScan

	// Scan, sync and graph share one slot so that no two of them run at once.
	busy        bool
	scanWanted  bool
	syncWanted  bool
	graphWanted bool
	// withLimits reads the usage limits when the queued graph starts,
	// so that changes read the limits no more often than the graph.
	withLimits bool
	lastGraph  time.Time
	graphReady <-chan time.Time

	usageRunning bool
	usageWanted  bool
	usageTick    <-chan time.Time

	syncDelay time.Duration
	syncTick  <-chan time.Time
}

// Run watches the local logs, syncs Cursor and reads usage until ctx is canceled.
// It returns after every watcher and tokscale process has stopped.
func (r *Reader) Run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	l := &loop{r: r, ctx: ctx, results: make(chan result), syncDelay: pollInterval, changed: make(chan struct{}, 1)}
	defer l.wg.Wait()
	defer cancel()
	if err := os.MkdirAll(r.configDir, 0o700); err != nil {
		r.logger.Warn("local_usage_unavailable", "cause", err)
		return
	}

	// The saved scan locations spare the costly `clients --json` at each start.
	if saved, err := loadScan(r.scanFile); err == nil {
		l.known = set(saved.Clients)
		l.watch(saved.Paths)
	} else {
		if !errors.Is(err, fs.ErrNotExist) {
			r.logger.Warn("local_usage_scan_unreadable", "cause", err)
		}
		l.scanWanted = true
	}

	// At start, the Cursor sync decides whether to keep syncing, and the graph follows it.
	l.syncWanted, l.graphWanted = true, true
	l.requestUsage()
	l.dispatch()
	var settle, graphRetry <-chan time.Time
	midnight := time.After(untilMidnight(time.Now()))
	for {
		select {
		case <-ctx.Done():
			return
		case <-l.changed:
			if settle == nil {
				settle = time.After(settleDelay)
			}
		case <-settle:
			settle = nil
			l.graphWanted, l.withLimits = true, true
		case <-l.graphReady:
			l.graphReady = nil
		case <-graphRetry:
			graphRetry = nil
			l.graphWanted = true
		case <-midnight:
			midnight = time.After(untilMidnight(time.Now()))
			l.graphWanted = true
		case <-l.usageTick:
			l.requestUsage()
		case <-l.syncTick:
			l.syncTick = nil
			l.syncWanted = true
		case res := <-l.results:
			switch res.job {
			case graphJob:
				l.busy = false
				if res.err != nil {
					r.logger.Warn("local_usage_graph_failed", "cause", res.err)
					graphRetry = time.After(pollInterval)
				} else {
					l.periods = &res.periods
					l.publish()
					l.checkClients(res.clients)
				}
			case usageJob:
				l.usageRunning = false
				if res.err != nil {
					r.logger.Warn("local_usage_limits_failed", "cause", res.err)
				} else {
					l.limits = &res.limits
					l.publish()
				}
				if l.usageWanted {
					l.usageWanted = false
					l.requestUsage()
				}
			case syncJob:
				l.busy = false
				l.handleSync(res)
			case scanJob:
				l.busy = false
				if res.err != nil {
					r.logger.Warn("local_usage_scan_failed", "cause", res.err)
					l.afterScan = scanAdopt
				} else {
					l.watch(res.paths)
					l.afterScan = scanSave
				}
				l.graphWanted = true
			}
		}
		l.dispatch()
	}
}

// requestUsage starts reading usage limits, or queues one read while another runs.
// The next periodic read counts from this start.
func (l *loop) requestUsage() {
	if l.usageRunning {
		l.usageWanted = true
		return
	}
	l.usageRunning = true
	l.usageTick = time.After(pollInterval)
	l.start(func(ctx context.Context) result {
		limits, err := l.r.readLimits(ctx)
		return result{job: usageJob, limits: limits, err: err}
	})
}

// dispatch starts the queued scan, sync or graph when the slot is free, in that order.
func (l *loop) dispatch() {
	if l.busy {
		return
	}
	if l.scanWanted {
		l.scanWanted, l.busy = false, true
		l.start(func(ctx context.Context) result {
			paths, err := l.r.watchPaths(ctx)
			return result{job: scanJob, paths: paths, err: err}
		})
		return
	}
	if l.syncWanted {
		l.syncWanted, l.busy = false, true
		l.start(func(ctx context.Context) result {
			outcome, err := l.r.syncCursor(ctx)
			return result{job: syncJob, cursor: outcome, err: err}
		})
		return
	}
	if !l.graphWanted {
		return
	}
	if wait := graphInterval - time.Since(l.lastGraph); wait > 0 {
		if l.graphReady == nil {
			l.graphReady = time.After(wait)
		}
		return
	}
	l.graphWanted, l.busy = false, true
	l.lastGraph = time.Now()
	if l.withLimits {
		l.withLimits = false
		l.requestUsage()
	}
	l.start(func(ctx context.Context) result {
		periods, clients, err := l.r.readPeriods(ctx)
		return result{job: graphJob, periods: periods, clients: clients, err: err}
	})
}

// checkClients saves or adopts the clients of the graph that follows a scan. Otherwise a
// tool that is not known means a new tool, whose scan locations a new scan finds.
func (l *loop) checkClients(clients []string) {
	switch l.afterScan {
	case scanSave:
		if err := saveScan(l.r.scanFile, scan{Paths: l.paths, Clients: clients}); err != nil {
			l.r.logger.Warn("local_usage_scan_save_failed", "cause", err)
		}
		fallthrough
	case scanAdopt:
		l.known, l.afterScan = set(clients), scanDone
		return
	}
	for _, client := range clients {
		if !l.known[client] {
			l.r.logger.Info("local_usage_new_client", "client", client)
			l.scanWanted = true
			return
		}
	}
}

// watch replaces the watched scan locations with paths.
func (l *loop) watch(paths []string) {
	if l.stopWatch != nil {
		l.stopWatch()
	}
	ctx, stop := context.WithCancel(l.ctx)
	l.stopWatch, l.paths = stop, paths
	notify := func() {
		select {
		case l.changed <- struct{}{}:
		default:
		}
	}
	for _, path := range paths {
		l.wg.Go(func() {
			if err := watch(ctx, path, notify); err != nil && ctx.Err() == nil {
				l.r.logger.Warn("local_usage_watch_stopped", "path", path, "cause", err)
			}
		})
	}
}

func set(values []string) map[string]bool {
	out := make(map[string]bool, len(values))
	for _, v := range values {
		out[v] = true
	}
	return out
}

func (l *loop) handleSync(res result) {
	logger := l.r.logger
	switch res.cursor {
	case cursorSynced:
		if res.err != nil {
			logger.Warn("cursor_sync_partial", "cause", res.err)
		}
		l.syncDelay = pollInterval
		l.syncTick = time.After(pollInterval)
		l.graphWanted, l.withLimits = true, true
	case cursorUnused:
		// Cursor is not signed in. A restart checks again.
		logger.Info("cursor_sync_disabled", "cause", res.err)
	case cursorExpired:
		logger.Warn("cursor_sync_disabled", "cause", res.err)
	case cursorFailed:
		logger.Warn("cursor_sync_failed", "cause", res.err, "retry", l.syncDelay)
		l.syncTick = time.After(l.syncDelay)
		l.syncDelay = min(2*l.syncDelay, maxSyncDelay)
	}
}

// publish replaces the state once both the periods and the limits have arrived,
// unless nothing has changed.
func (l *loop) publish() {
	if l.periods == nil || l.limits == nil {
		return
	}
	stats := &usage.Stats{Periods: *l.periods, Limits: *l.limits}
	if reflect.DeepEqual(stats, l.published) {
		return
	}
	l.published = stats
	l.r.state.Set(stats)
}

func (l *loop) start(run func(context.Context) result) {
	l.wg.Go(func() {
		res := run(l.ctx)
		select {
		case l.results <- res:
		case <-l.ctx.Done():
		}
	})
}

func untilMidnight(now time.Time) time.Duration {
	y, m, d := now.Date()
	return time.Date(y, m, d+1, 0, 0, 1, 0, now.Location()).Sub(now)
}

func (r *Reader) run(ctx context.Context, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, r.executable, args...)
	command.Env = append(os.Environ(), "TOKSCALE_CONFIG_DIR="+r.configDir)
	command.Stderr = io.Discard
	hideWindow(command)
	output, err := command.Output()
	if err != nil {
		return nil, fmt.Errorf("tokscale %v: %w", args, err)
	}
	return output, nil
}
