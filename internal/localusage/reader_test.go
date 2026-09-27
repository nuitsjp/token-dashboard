package localusage

import (
	"encoding/json"
	"testing"
	"time"
)

func TestConvertTokscalePeriods(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  int64
		cost  float64
	}{
		{name: "today", input: `{"totalInput":101,"totalOutput":202,"totalCacheRead":303,"totalCacheWrite":404,"totalCost":1.25}`, want: 1010, cost: 1.25},
		{name: "month", input: `{"totalInput":1001,"totalOutput":2002,"totalCacheRead":3003,"totalCacheWrite":4004,"totalCost":12.5}`, want: 10010, cost: 12.5},
		{name: "all", input: `{"totalInput":10001,"totalOutput":20002,"totalCacheRead":30003,"totalCacheWrite":40004,"totalCost":125.75}`, want: 100010, cost: 125.75},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var raw period
			if err := json.Unmarshal([]byte(tc.input), &raw); err != nil {
				t.Fatal(err)
			}
			got := convertPeriod(raw)
			if got.TotalTokens != tc.want || got.CostUSD != tc.cost {
				t.Fatalf("period = %+v, want tokens %d and cost %v", got, tc.want, tc.cost)
			}
		})
	}
}

func TestConvertTokscaleQuotas(t *testing.T) {
	const input = `[
  {
    "provider": "OpenAI",
    "plan": "Plus",
    "email": "user@example.com",
    "metrics": [
      {"label": "5-hour", "used_percent": 23.5, "remaining_percent": 76.5, "resets_at": "2026-09-28T12:34:56Z"},
      {"label": "weekly", "resets_at": "2026-10-02"}
    ]
  }
]`
	var raw []provider
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		t.Fatal(err)
	}
	got, err := convertProviders(raw)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Providers) != 1 {
		t.Fatalf("providers = %+v", got.Providers)
	}
	p := got.Providers[0]
	if p.Provider != "OpenAI" || p.PlanLabel != "Plus" || p.AccountLabel != "user@example.com" || len(p.Windows) != 2 {
		t.Fatalf("provider = %+v", p)
	}

	window := p.Windows[0]
	if !window.ShowMeter || window.UsedPercent == nil || *window.UsedPercent != 23.5 || window.RemainingPercent == nil || *window.RemainingPercent != 76.5 {
		t.Fatalf("meter window = %+v", window)
	}
	wantReset := time.Date(2026, 9, 28, 12, 34, 56, 0, time.UTC)
	if window.ResetsAt == nil || !window.ResetsAt.Equal(wantReset) {
		t.Fatalf("RFC3339 reset = %v, want %v", window.ResetsAt, wantReset)
	}

	window = p.Windows[1]
	if window.ShowMeter || window.UsedPercent != nil || window.RemainingPercent != nil {
		t.Fatalf("unmetered window = %+v", window)
	}
	wantDateOnlyReset := time.Date(2026, 10, 2, 0, 0, 0, 0, time.Local)
	if window.ResetsAt == nil || !window.ResetsAt.Equal(wantDateOnlyReset) {
		t.Fatalf("date-only reset = %v, want %v", window.ResetsAt, wantDateOnlyReset)
	}
}
