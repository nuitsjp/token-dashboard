package display

import (
	"slices"
	"strings"

	"token-monitor-turzx/internal/usage"
)

// LimitWindow is one window in the Usage Limits list of the window.
type LimitWindow struct {
	// Key identifies the window across redraws.
	Key              string   `json:"key"`
	Name             string   `json:"name"`
	RemainingPercent *float64 `json:"remainingPercent"`
	Shown            bool     `json:"shown"`
}

// LimitContract is one contract and the windows that can show a meter, in the order the source sent them.
type LimitContract struct {
	Provider string        `json:"provider"`
	Plan     string        `json:"plan"`
	Windows  []LimitWindow `json:"windows"`
}

// windowKey identifies a window by provider, account, kind and label. The unit separator cannot
// appear in a label.
func windowKey(p usage.Provider, w usage.Window) string {
	return strings.Join([]string{p.Provider, p.AccountLabel, w.Kind, w.Label}, "\x1f")
}

// contractsOf lists the contracts that have a window with a meter, marking the hidden windows.
func contractsOf(stats *usage.Stats, hidden map[string]bool) []LimitContract {
	out := []LimitContract{}
	if stats == nil {
		return out
	}
	for _, p := range stats.Limits.Providers {
		c := LimitContract{Provider: p.Provider, Plan: p.PlanLabel}
		if c.Plan == "" {
			c.Plan = p.AccountLabel
		}
		for _, w := range p.Windows {
			if !w.ShowMeter {
				continue
			}
			key := windowKey(p, w)
			name := w.Label
			if name == "" {
				name = w.Kind
			}
			c.Windows = append(c.Windows, LimitWindow{Key: key, Name: name, RemainingPercent: w.RemainingPercent, Shown: !hidden[key]})
		}
		if len(c.Windows) > 0 {
			out = append(out, c)
		}
	}
	return out
}

// withoutHidden returns stats as if the hidden windows had not been reported. It copies what it
// changes because the latest stats are shared with the reader.
func withoutHidden(stats *usage.Stats, hidden map[string]bool) *usage.Stats {
	if stats == nil || len(hidden) == 0 {
		return stats
	}
	out := *stats
	out.Limits.Providers = nil
	for _, p := range stats.Limits.Providers {
		p.Windows = slices.DeleteFunc(slices.Clone(p.Windows), func(w usage.Window) bool { return hidden[windowKey(p, w)] })
		out.Limits.Providers = append(out.Limits.Providers, p)
	}
	return &out
}
