package localusage

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"token-monitor-turzx/internal/usage"
)

type contribution struct {
	Date   string `json:"date"`
	Totals struct {
		Cost float64 `json:"cost"`
	} `json:"totals"`
	TokenBreakdown struct {
		Input      int64 `json:"input"`
		Output     int64 `json:"output"`
		CacheRead  int64 `json:"cacheRead"`
		CacheWrite int64 `json:"cacheWrite"`
	} `json:"tokenBreakdown"`
}

type graph struct {
	Summary struct {
		Clients []string `json:"clients"`
	} `json:"summary"`
	Contributions []contribution `json:"contributions"`
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

type scanPath struct {
	Path   string `json:"path"`
	Exists bool   `json:"exists"`
}

type clients struct {
	Clients []struct {
		Client          string     `json:"client"`
		SessionsPath    string     `json:"sessionsPath"`
		AdditionalPaths []scanPath `json:"additionalPaths"`
		HeadlessPaths   []scanPath `json:"headlessPaths"`
		LegacyPaths     []scanPath `json:"legacyPaths"`
	} `json:"clients"`
}

type cursorOutcome int

const (
	cursorSynced cursorOutcome = iota
	// cursorUnused means no Cursor account is signed in.
	cursorUnused
	// cursorExpired means the saved session no longer works.
	cursorExpired
	// cursorFailed is any other failure, such as a network error.
	cursorFailed
)

// readPeriods reads Today, Month and All from the daily totals of one graph run,
// with the tools that have usage.
func (r *Reader) readPeriods(ctx context.Context) (usage.Periods, []string, error) {
	output, err := r.run(ctx, "graph", "--no-spinner")
	if err != nil {
		return usage.Periods{}, nil, err
	}
	var g graph
	if err := json.Unmarshal(output, &g); err != nil {
		return usage.Periods{}, nil, fmt.Errorf("parse graph: %w", err)
	}
	return convertGraph(g, time.Now()), g.Summary.Clients, nil
}

// convertGraph sums the days of now's date and month in now's time zone, the same as tokscale.
// The tokens leave out reasoning, as the Hub does.
func convertGraph(g graph, now time.Time) usage.Periods {
	today := now.Format("2006-01-02")
	month := today[:len("2006-01-")]
	var periods usage.Periods
	for _, c := range g.Contributions {
		b := c.TokenBreakdown
		day := usage.Period{TotalTokens: b.Input + b.Output + b.CacheRead + b.CacheWrite, CostUSD: c.Totals.Cost}
		add(&periods.AllTime, day)
		if strings.HasPrefix(c.Date, month) {
			add(&periods.Month, day)
		}
		if c.Date == today {
			add(&periods.Today, day)
		}
	}
	return periods
}

func add(total *usage.Period, day usage.Period) {
	total.TotalTokens += day.TotalTokens
	total.CostUSD += day.CostUSD
}

func (r *Reader) readLimits(ctx context.Context) (usage.Limits, error) {
	output, err := r.run(ctx, "usage", "--json")
	if err != nil {
		return usage.Limits{}, err
	}
	var providers []provider
	if err := json.Unmarshal(output, &providers); err != nil {
		return usage.Limits{}, fmt.Errorf("parse usage limits: %w", err)
	}
	return convertProviders(providers)
}

// syncCursor syncs the Cursor usage cache. The command exits with 0 even when the sync fails,
// so the outcome comes from its JSON. A partial sync is cursorSynced with an error.
func (r *Reader) syncCursor(ctx context.Context) (cursorOutcome, error) {
	output, err := r.run(ctx, "cursor", "sync", "--json")
	if err != nil {
		return cursorFailed, err
	}
	var sync struct {
		Synced bool    `json:"synced"`
		Error  *string `json:"error"`
	}
	if err := json.Unmarshal(output, &sync); err != nil {
		return cursorFailed, fmt.Errorf("parse cursor sync: %w", err)
	}
	var cause error
	if sync.Error != nil {
		cause = errors.New(*sync.Error)
	}
	switch {
	case sync.Synced:
		return cursorSynced, cause
	case cause == nil:
		return cursorFailed, errors.New("cursor sync failed")
	case *sync.Error == "Not authenticated":
		return cursorUnused, cause
	case strings.Contains(*sync.Error, "Cursor session expired"):
		return cursorExpired, cause
	default:
		return cursorFailed, cause
	}
}

// watchPaths returns the existing scan locations of every client but Cursor,
// whose cache only changes through the sync.
func (r *Reader) watchPaths(ctx context.Context) ([]string, error) {
	output, err := r.run(ctx, "clients", "--json")
	if err != nil {
		return nil, err
	}
	var c clients
	if err := json.Unmarshal(output, &c); err != nil {
		return nil, fmt.Errorf("parse clients: %w", err)
	}
	var paths []string
	seen := map[string]bool{}
	for _, client := range c.Clients {
		if client.Client == "cursor" {
			continue
		}
		candidates := []string{client.SessionsPath}
		for _, list := range [][]scanPath{client.AdditionalPaths, client.HeadlessPaths, client.LegacyPaths} {
			for _, p := range list {
				candidates = append(candidates, p.Path)
			}
		}
		for _, path := range candidates {
			if path == "" || seen[path] {
				continue
			}
			if info, err := os.Stat(path); err == nil && info.IsDir() {
				seen[path] = true
				paths = append(paths, path)
			}
		}
	}
	return paths, nil
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
				Kind:             kindOf(m.Label),
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

// kindOf is the window kind named by the last word of a tokscale label, so that the display can look
// up the window length like it does for the Hub. Other labels have no kind.
func kindOf(label string) string {
	l := strings.ToLower(strings.TrimSpace(label))
	for suffix, kind := range map[string]string{"5-hour": "session", "5h": "session", "session": "session", "daily": "daily", "weekly": "weekly", "monthly": "billing"} {
		if strings.HasSuffix(l, suffix) {
			return kind
		}
	}
	return ""
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
