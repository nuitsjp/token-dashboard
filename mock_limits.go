package main

import (
	"os"
	"time"

	"token-monitor-turzx/internal/usage"
)

// mockLimits is the stage 2 mock of "表示する利用枠を選ぶ": the server build shows the fixed usage
// below instead of reading a source. It is removed in stage 4.
func mockLimits() bool {
	return serverMode && os.Getenv("WAILS_MOCK_LIMITS") == "1"
}

func mockStats(now time.Time) *usage.Stats {
	at := func(d time.Duration) *time.Time { t := now.Add(d); return &t }
	pct := func(v float64) *float64 { return &v }
	win := func(kind, label string, remaining float64, reset time.Duration) usage.Window {
		return usage.Window{Kind: kind, Label: label, ShowMeter: true, RemainingPercent: pct(remaining), ResetsAt: at(reset)}
	}
	contract := func(provider, account, plan string, ws ...usage.Window) usage.Provider {
		return usage.Provider{Provider: provider, AccountLabel: account, PlanLabel: plan, Windows: ws}
	}
	const h = time.Hour
	return &usage.Stats{
		Periods: usage.Periods{
			Today:   usage.Period{TotalTokens: 31_204_552, CostUSD: 18.42},
			Month:   usage.Period{TotalTokens: 1_284_330_117, CostUSD: 902.17},
			AllTime: usage.Period{TotalTokens: 25_984_802_353, CostUSD: 14849.73},
		},
		Limits: usage.Limits{Providers: []usage.Provider{
			contract("Claude", "work@example.com", "Max 20x", win("session", "5-hour", 48, 3*h), win("weekly", "Weekly", 82, 90*h), win("weekly", "Weekly Sonnet", 91, 90*h)),
			contract("Claude", "me@example.com", "Pro", win("session", "5-hour", 12, 1*h), win("weekly", "Weekly", 35, 40*h)),
			contract("Codex", "work@example.com", "Pro", win("session", "5-hour", 87, 4*h), win("weekly", "Weekly", 64, 120*h)),
			contract("Cursor", "work@example.com", "Pro", win("billing", "Monthly", 71, 400*h)),
			contract("Gemini", "me@example.com", "Advanced", win("daily", "Daily", 95, 9*h)),
			contract("Grok", "me@example.com", "SuperGrok", win("daily", "Daily", 100, 12*h), win("weekly", "Weekly", 97, 150*h)),
			contract("Antigravity", "me@example.com", "Pro", win("session", "5-hour", 100, 5*h), win("weekly", "Weekly", 99, 160*h)),
			contract("OpenCode", "me@example.com", "Go", win("session", "5-hour", 89, 2*h), win("weekly", "Weekly", 76, 100*h), win("billing", "Monthly", 70, 500*h)),
		}},
	}
}
