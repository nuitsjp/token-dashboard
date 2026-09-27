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
			{Provider: "antigravity", PlanLabel: "Pro", Windows: []usage.Window{
				{Kind: "session", Label: "Claude/GPT 5-hour", ShowMeter: true, RemainingPercent: pct(100), ResetsAt: at(4*time.Hour + 55*time.Minute)},
				{Kind: "session", Label: "Gemini 5-hour", ShowMeter: true, RemainingPercent: pct(96), ResetsAt: at(1*time.Hour + 34*time.Minute)},
				{Kind: "weekly", Label: "Claude/GPT weekly", ShowMeter: true, RemainingPercent: pct(100), ResetsAt: at(6*24*time.Hour + 19*time.Hour)},
				{Kind: "weekly", Label: "Gemini weekly", ShowMeter: true, RemainingPercent: pct(99), ResetsAt: at(6*24*time.Hour + 15*time.Hour)},
			}},
			{Provider: "claude", PlanLabel: "Pro", Windows: []usage.Window{
				{Kind: "session", Label: "session", ShowMeter: true, RemainingPercent: pct(72), ResetsAt: at(4*time.Hour + 20*time.Minute)},
				{Kind: "weekly", Label: "weekly", ShowMeter: true, RemainingPercent: pct(85), ResetsAt: at(6*24*time.Hour + 18*time.Hour)},
				{Kind: "billing", Label: "Usage credits", ShowMeter: false},
			}},
			{Provider: "codex", PlanLabel: "Pro 5x", Windows: []usage.Window{
				{Kind: "weekly", Label: "weekly", ShowMeter: true, RemainingPercent: pct(38), ResetsAt: at(6*24*time.Hour + 15*time.Hour + 40*time.Minute)},
			}},
			{Provider: "cursor", PlanLabel: "Pro", Windows: []usage.Window{
				{Kind: "billing", Label: "Cursor Models", ShowMeter: true, RemainingPercent: pct(95), ResetsAt: at(15*24*time.Hour + 17*time.Hour)},
				{Kind: "billing", Label: "Other Models", ShowMeter: true, RemainingPercent: pct(71), ResetsAt: at(15*24*time.Hour + 17*time.Hour)},
				{Kind: "billing", Label: "Grok Bot", ShowMeter: true, RemainingPercent: pct(100)},
			}},
			{Provider: "grok", PlanLabel: "SuperGrok", Windows: []usage.Window{
				{Kind: "weekly", Label: "Weekly", ShowMeter: true, RemainingPercent: pct(91), ResetsAt: at(5*24*time.Hour + 3*time.Hour)},
			}},
			{Provider: "opencode", PlanLabel: "Go", Windows: []usage.Window{
				{Kind: "billing", Label: "billing", ShowMeter: true, RemainingPercent: pct(12), ResetsAt: at(13*24*time.Hour + 17*time.Hour)},
				{Kind: "session", Label: "session", ShowMeter: true, RemainingPercent: pct(100), ResetsAt: at(4*time.Hour + 55*time.Minute)},
				{Kind: "weekly", Label: "weekly", ShowMeter: true, RemainingPercent: pct(97), ResetsAt: at(21 * time.Hour)},
				{Kind: "billing", Label: "Credits", ShowMeter: false},
			}},
		}},
	}
}
