// Package hub receives the latest statistics from a Token Monitor Hub over SSE.
package hub

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"token-monitor-turzx/internal/usage"
)

const (
	firstDelay = time.Second
	maxDelay   = 60 * time.Second
	// The Hub sends a heartbeat every 30 seconds; silence beyond this means the stream stalled.
	idleTimeout = 90 * time.Second
)

// Receiver keeps one stream open and puts every snapshot and stats into the state.
type Receiver struct {
	connection func() (url, token string, err error)
	state      *usage.State
	logger     *slog.Logger
	client     *http.Client
	restart    chan struct{}
}

func New(connection func() (string, string, error), state *usage.State, logger *slog.Logger) *Receiver {
	return &Receiver{connection: connection, state: state, logger: logger, client: &http.Client{}, restart: make(chan struct{}, 1)}
}

// Restart reconnects with the current connection settings, e.g. after they were saved.
func (r *Receiver) Restart() {
	select {
	case r.restart <- struct{}{}:
	default:
	}
}

// Run receives until ctx ends. After a failure it waits from 1 second, doubling up to 60 seconds.
func (r *Receiver) Run(ctx context.Context) {
	delay := firstDelay
	for {
		received, err := r.receive(ctx)
		if ctx.Err() != nil {
			return
		}
		if received {
			delay = firstDelay
		}
		var wait time.Duration
		switch {
		case err == nil: // restarted on request
		case errors.Is(err, errNotConfigured):
			wait = -1 // until the settings are saved
		default:
			r.logger.Warn("hub_receive_failed", "cause", err, "retryIn", delay.String())
			wait, delay = delay, min(delay*2, maxDelay)
		}
		if !r.sleep(ctx, wait) {
			return
		}
	}
}

// sleep waits for wait (forever when negative) or a restart request. It is false once ctx ends.
func (r *Receiver) sleep(ctx context.Context, wait time.Duration) bool {
	var timer <-chan time.Time
	if wait >= 0 {
		timer = time.After(wait)
	}
	select {
	case <-ctx.Done():
		return false
	case <-r.restart:
	case <-timer:
	}
	return true
}

var errNotConfigured = errors.New("hub connection is not configured")

// receive reads one stream until it fails, stalls or a restart is requested (nil error).
// received reports whether a snapshot or stats arrived. Errors never contain the URL or token.
func (r *Receiver) receive(ctx context.Context) (received bool, err error) {
	base, token, err := r.connection()
	if err != nil {
		return false, fmt.Errorf("settings: %w", err)
	}
	if base == "" || token == "" {
		return false, errNotConfigured
	}
	ctx, cancel := context.WithCancelCause(ctx)
	defer cancel(nil)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/stats/stream", nil)
	if err != nil {
		return false, errors.New("invalid hub URL")
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("x-token-monitor-stream", "2")
	idle := time.AfterFunc(idleTimeout, func() { cancel(errStalled) })
	defer idle.Stop()
	go func() {
		select {
		case <-r.restart:
			cancel(errRestart)
		case <-ctx.Done():
		}
	}()
	resp, err := r.client.Do(req)
	if err != nil {
		return false, streamError(ctx, "connect failed")
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 64*1024*1024)
	var event string
	var data strings.Builder
	for scanner.Scan() {
		idle.Reset(idleTimeout)
		line := scanner.Text()
		switch {
		case line == "":
			if event == "snapshot" || event == "stats" {
				var msg struct {
					Stats *usage.Stats `json:"stats"`
				}
				if err := json.Unmarshal([]byte(data.String()), &msg); err != nil || msg.Stats == nil {
					return received, fmt.Errorf("invalid %s event", event)
				}
				r.state.Set(msg.Stats)
				received = true
			}
			event = ""
			data.Reset()
		case strings.HasPrefix(line, "event:"):
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
		case strings.HasPrefix(line, "data:"):
			if data.Len() > 0 {
				data.WriteByte('\n')
			}
			data.WriteString(strings.TrimPrefix(strings.TrimPrefix(line, "data:"), " "))
		}
	}
	return received, streamError(ctx, "stream ended")
}

var (
	errRestart = errors.New("restart requested")
	errStalled = errors.New("stream stalled")
)

// streamError says why the stream ended without the request URL, which net/http errors include.
// A requested restart is not an error.
func streamError(ctx context.Context, fallback string) error {
	switch cause := context.Cause(ctx); {
	case errors.Is(cause, errRestart):
		return nil
	case errors.Is(cause, errStalled):
		return cause
	default:
		return errors.New(fallback)
	}
}
