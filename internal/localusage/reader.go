// Package localusage reads local token usage from tokscale.
package localusage

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os/exec"
	"time"

	"token-monitor-turzx/internal/usage"
)

const refreshInterval = 5 * time.Minute

// Reader periodically reads usage from a local tokscale executable.
type Reader struct {
	executable string
	state      *usage.State
	logger     *slog.Logger
}

// New creates a local tokscale reader.
func New(executable string, state *usage.State, logger *slog.Logger) *Reader {
	return &Reader{executable: executable, state: state, logger: logger}
}

// Run reads immediately and then once every five minutes until ctx is canceled.
func (r *Reader) Run(ctx context.Context) {
	ticker := time.NewTicker(refreshInterval)
	defer ticker.Stop()
	for {
		if err := r.update(ctx); err != nil && ctx.Err() == nil {
			r.logger.Warn("local_usage_update_failed", "cause", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

type period struct {
	TotalInput      int64   `json:"totalInput"`
	TotalOutput     int64   `json:"totalOutput"`
	TotalCacheRead  int64   `json:"totalCacheRead"`
	TotalCacheWrite int64   `json:"totalCacheWrite"`
	TotalCost       float64 `json:"totalCost"`
}

type metric struct {
	Label            string   `json:"label"`
	UsedPercent      *float64 `json:"used_percent"`
	RemainingPercent *float64 `json:"remaining_percent"`
	ResetsAt         *string  `json:"resets_at"`
}

type provider struct {
	Provider string   `json:"provider"`
	Plan     string   `json:"plan"`
	Email    string   `json:"email"`
	Metrics  []metric `json:"metrics"`
}

func (r *Reader) update(ctx context.Context) error {
	commands := [4][]string{
		{"--today", "--json", "--no-spinner"},
		{"--month", "--json", "--no-spinner"},
		{"--json", "--no-spinner"},
		{"usage", "--json"},
	}
	outputs := make([][]byte, len(commands))
	var firstErr error
	for i, args := range commands {
		output, err := r.run(ctx, args...)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		outputs[i] = output
	}
	if firstErr != nil {
		return firstErr
	}

	var today, month, allTime period
	if err := json.Unmarshal(outputs[0], &today); err != nil {
		return fmt.Errorf("parse today usage: %w", err)
	}
	if err := json.Unmarshal(outputs[1], &month); err != nil {
		return fmt.Errorf("parse month usage: %w", err)
	}
	if err := json.Unmarshal(outputs[2], &allTime); err != nil {
		return fmt.Errorf("parse all-time usage: %w", err)
	}
	var providers []provider
	if err := json.Unmarshal(outputs[3], &providers); err != nil {
		return fmt.Errorf("parse usage limits: %w", err)
	}

	limits, err := convertProviders(providers)
	if err != nil {
		return err
	}
	r.state.Set(&usage.Stats{
		Periods: usage.Periods{
			Today:   convertPeriod(today),
			Month:   convertPeriod(month),
			AllTime: convertPeriod(allTime),
		},
		Limits: limits,
	})
	return nil
}

func (r *Reader) run(ctx context.Context, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, r.executable, args...)
	command.Stderr = io.Discard
	return command.Output()
}

func convertPeriod(p period) usage.Period {
	return usage.Period{
		TotalTokens: p.TotalInput + p.TotalOutput + p.TotalCacheRead + p.TotalCacheWrite,
		CostUSD:     p.TotalCost,
	}
}

func convertProviders(providers []provider) (usage.Limits, error) {
	out := usage.Limits{Providers: make([]usage.Provider, 0, len(providers))}
	for _, p := range providers {
		converted := usage.Provider{
			Provider:     p.Provider,
			PlanLabel:    p.Plan,
			AccountLabel: p.Email,
			Windows:      make([]usage.Window, 0, len(p.Metrics)),
		}
		for _, m := range p.Metrics {
			reset, err := parseReset(m.ResetsAt)
			if err != nil {
				return usage.Limits{}, err
			}
			converted.Windows = append(converted.Windows, usage.Window{
				Label:            m.Label,
				ShowMeter:        m.UsedPercent != nil || m.RemainingPercent != nil,
				UsedPercent:      m.UsedPercent,
				RemainingPercent: m.RemainingPercent,
				ResetsAt:         reset,
			})
		}
		out.Providers = append(out.Providers, converted)
	}
	return out, nil
}

func parseReset(value *string) (*time.Time, error) {
	if value == nil || *value == "" {
		return nil, nil
	}
	if parsed, err := time.Parse(time.RFC3339, *value); err == nil {
		return &parsed, nil
	}
	if parsed, err := time.ParseInLocation("2006-01-02", *value, time.Local); err == nil {
		return &parsed, nil
	}
	return nil, fmt.Errorf("parse resets_at: unsupported date format")
}
