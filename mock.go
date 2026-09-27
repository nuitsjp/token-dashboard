package main

import (
	"context"
	"time"

	"token-monitor-turzx/internal/usage"
)

// mockHub stands in for the Hub in dev:mock (stages 2 and 3; removed in stage 4): the first
// snapshot after 3 seconds, then a stats update every 15 seconds with Today's usage growing.
func mockHub(ctx context.Context, state *usage.State) {
	now := time.Now()
	delay := 3 * time.Second
	for i := 0; ; i++ {
		select {
		case <-ctx.Done():
			return
		case <-time.After(delay):
		}
		delay = 15 * time.Second
		state.Set(mockUsage(now, i))
	}
}

func mockUsage(now time.Time, update int) *usage.Stats {
	pct := func(v float64) *float64 { return &v }
	at := func(d time.Duration) *time.Time { t := now.Add(d); return &t }
	return &usage.Stats{
		Periods: usage.Periods{
			Today:   usage.Period{TotalTokens: 12_345_678 + int64(update)*123_456, CostUSD: 12.34 + float64(update)*0.12},
			Month:   usage.Period{TotalTokens: 345_678_901, CostUSD: 345.67},
			AllTime: usage.Period{TotalTokens: 2_345_678_901, CostUSD: 2345.67},
		},
		Limits: usage.Limits{Providers: []usage.Provider{
			{Provider: "claude", AccountLabel: "Claude Max", Windows: []usage.Window{
				{Kind: "session", Label: "Session", ShowMeter: true, RemainingPercent: pct(58), ResetsAt: at(2*time.Hour + 13*time.Minute)},
				{Kind: "weekly", Label: "Weekly", ShowMeter: true, RemainingPercent: pct(80), ResetsAt: at(3*24*time.Hour + 4*time.Hour)},
				{Kind: "billing", Label: "Usage credits", ShowMeter: false},
			}},
			{Provider: "codex", AccountLabel: "ChatGPT Pro", Windows: []usage.Window{
				{Kind: "session", Label: "5 hour", ShowMeter: true, RemainingPercent: pct(35), ResetsAt: at(47 * time.Minute)},
				{Kind: "weekly", Label: "Weekly", ShowMeter: true, RemainingPercent: pct(12), ResetsAt: at(6*24*time.Hour + 30*time.Minute)},
			}},
			{Provider: "copilot", AccountLabel: "GitHub Copilot Business", Windows: []usage.Window{
				{Kind: "billing", Label: "Premium requests", ShowMeter: true, RemainingPercent: pct(91), ResetsAt: at(18*24*time.Hour + 2*time.Hour)},
			}},
			{Provider: "gemini", AccountLabel: "Gemini Code Assist", Windows: []usage.Window{
				{Kind: "daily", Label: "Daily", ShowMeter: true, RemainingPercent: pct(67), ResetsAt: at(9*time.Hour + 5*time.Minute)},
			}},
			{Provider: "opencode", AccountLabel: "OpenCode Zen", Windows: []usage.Window{
				{Kind: "billing", Label: "Credits", ShowMeter: false},
			}},
			{Provider: "kimi", AccountLabel: "Kimi Membership", Windows: []usage.Window{
				{Kind: "billing", Label: "Monthly", ShowMeter: true, RemainingPercent: pct(44), ResetsAt: at(11*24*time.Hour + 7*time.Hour)},
				{Kind: "weekly", Label: "Weekly", ShowMeter: true, RemainingPercent: pct(73), ResetsAt: at(2*24*time.Hour + 16*time.Hour)},
				{Kind: "daily", Label: "Daily", ShowMeter: true, RemainingPercent: pct(5), ResetsAt: at(3*time.Hour + 58*time.Minute)},
			}},
		}},
	}
}
