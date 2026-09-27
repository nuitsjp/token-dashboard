package hub

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"token-monitor-turzx/internal/usage"
)

const snapshot = `{"type":"snapshot","stats":{"periods":{"today":{"totalTokens":%d,"costUsd":1.5}}}}`

func TestReceiverRetriesWithDoublingDelay(t *testing.T) {
	var mu sync.Mutex
	var attempts []time.Time
	hub := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		attempts = append(attempts, time.Now())
		n := len(attempts)
		mu.Unlock()
		if n < 3 {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		fmt.Fprintf(w, "event: snapshot\ndata: "+snapshot+"\n\n", 7)
		w.(http.Flusher).Flush()
		<-r.Context().Done()
	}))
	defer hub.Close()
	state := usage.NewState()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := New(func() (string, string, error) { return hub.URL, "token", nil }, state, slog.New(slog.NewTextHandler(io.Discard, nil)))
	go r.Run(ctx)
	select {
	case <-state.Changed():
	case <-time.After(10 * time.Second):
		t.Fatal("no snapshot")
	}
	if got := state.Latest().Periods.Today.TotalTokens; got != 7 {
		t.Fatalf("today tokens %d", got)
	}
	mu.Lock()
	defer mu.Unlock()
	if first, second := attempts[1].Sub(attempts[0]), attempts[2].Sub(attempts[1]); first < time.Second || second < 2*time.Second {
		t.Fatalf("delays %v, %v; want at least 1s then 2s", first, second)
	}
}

func TestReceiverReconnectsWithNewSettingsOnRestart(t *testing.T) {
	serve := func(tokens int) *httptest.Server {
		return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Header.Get("Authorization") != "Bearer token" || r.Header.Get("x-token-monitor-stream") != "2" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			fmt.Fprintf(w, "event: snapshot\ndata: "+snapshot+"\n\n", tokens)
			w.(http.Flusher).Flush()
			<-r.Context().Done()
		}))
	}
	first, second := serve(1), serve(2)
	defer first.Close()
	defer second.Close()
	var mu sync.Mutex
	url := first.URL
	state := usage.NewState()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	r := New(func() (string, string, error) { mu.Lock(); defer mu.Unlock(); return url, "token", nil }, state, slog.New(slog.NewTextHandler(io.Discard, nil)))
	go r.Run(ctx)
	wait := func(want int64) {
		t.Helper()
		deadline := time.After(5 * time.Second)
		for {
			if s := state.Latest(); s != nil && s.Periods.Today.TotalTokens == want {
				return
			}
			select {
			case <-state.Changed():
			case <-deadline:
				t.Fatalf("today tokens never became %d", want)
			}
		}
	}
	wait(1)
	mu.Lock()
	url = second.URL
	mu.Unlock()
	r.Restart()
	wait(2)
}
