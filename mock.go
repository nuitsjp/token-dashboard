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
	day := 24 * time.Hour
	return &usage.Stats{
		Periods: usage.Periods{
			Today:   usage.Period{TotalTokens: 265_038_583 + int64(update)*123_456, CostUSD: 124.01 + float64(update)*0.12},
			Month:   usage.Period{TotalTokens: 6_477_985_313, CostUSD: 2643.24},
			AllTime: usage.Period{TotalTokens: 25_998_505_023, CostUSD: 14855.81},
		},
		Limits: usage.Limits{Providers: []usage.Provider{
			{Provider: "antigravity", PlanLabel: "Pro", Windows: []usage.Window{
				{Kind: "session", Label: "Gemini 5-hour", ShowMeter: true, RemainingPercent: pct(100), ResetsAt: at(4*time.Hour + 59*time.Minute)},
				{Kind: "weekly", Label: "Gemini weekly", ShowMeter: true, RemainingPercent: pct(99), ResetsAt: at(6*day + 12*time.Hour)},
				{Kind: "session", Label: "Claude/GPT 5-hour", ShowMeter: true, RemainingPercent: pct(100), ResetsAt: at(4*time.Hour + 59*time.Minute)},
				{Kind: "weekly", Label: "Claude/GPT weekly", ShowMeter: true, RemainingPercent: pct(100), ResetsAt: at(6*day + 23*time.Hour)},
			}},
			{Provider: "claude", PlanLabel: "Pro", Windows: []usage.Window{
				{Kind: "session", Label: "session", ShowMeter: true, RemainingPercent: pct(48), ResetsAt: at(53 * time.Minute)},
				{Kind: "weekly", Label: "weekly", ShowMeter: true, RemainingPercent: pct(82), ResetsAt: at(6*day + 14*time.Hour)},
				{Kind: "billing", Label: "Usage credits", ShowMeter: false},
			}},
			{Provider: "codex", PlanLabel: "Pro 5x", Windows: []usage.Window{
				{Kind: "weekly", Label: "weekly", ShowMeter: true, RemainingPercent: pct(87), ResetsAt: at(6*day + 12*time.Hour)},
			}},
			{Provider: "cursor", PlanLabel: "Pro", Windows: []usage.Window{
				{Kind: "billing", Label: "Cursor Models", ShowMeter: true, RemainingPercent: pct(95), ResetsAt: at(15*day + 14*time.Hour)},
				{Kind: "billing", Label: "Other Models", ShowMeter: true, RemainingPercent: pct(71), ResetsAt: at(15*day + 14*time.Hour)},
				{Kind: "billing", Label: "Grok Bot", ShowMeter: true, RemainingPercent: pct(100)},
			}},
			{Provider: "grok", PlanLabel: "SuperGrok", Windows: []usage.Window{
				{Kind: "weekly", Label: "Weekly", ShowMeter: true, RemainingPercent: pct(91), ResetsAt: at(5*day + 3*time.Hour)},
			}},
			{Provider: "opencode", PlanLabel: "Go", Windows: []usage.Window{
				{Kind: "session", Label: "session", ShowMeter: true, RemainingPercent: pct(100), ResetsAt: at(4*time.Hour + 59*time.Minute)},
				{Kind: "weekly", Label: "weekly", ShowMeter: true, RemainingPercent: pct(97), ResetsAt: at(17*time.Hour + 33*time.Minute)},
				{Kind: "billing", Label: "billing", ShowMeter: true, RemainingPercent: pct(89), ResetsAt: at(13*day + 13*time.Hour)},
				{Kind: "billing", Label: "Credits", ShowMeter: false},
			}},
			{Provider: "copilot", PlanLabel: "Business", Windows: []usage.Window{
				{Kind: "billing", Label: "premium requests", ShowMeter: true, ResetsAt: at(4*day + 9*time.Hour)},
			}},
		}},
	}
}
