package localusage

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"token-monitor-turzx/internal/usage"
)

func TestClientPeriodsKeepToolsAndMissingValuesSeparate(t *testing.T) {
	const input = `{"summary":{"clients":["summary-only"]},"contributions":[
 {"date":"2026-09-30","totals":{"cost":30},"tokenBreakdown":{"input":3000},"clients":[{"client":"claude","tokens":{"input":30},"cost":3},{"client":"partial","tokens":{"input":9},"cost":0.5}]},
 {"date":"2026-10-01","totals":{"cost":20},"tokenBreakdown":{"input":2000},"clients":[{"client":"claude","tokens":{"input":20},"cost":2},{"client":"partial","tokens":null,"cost":null}]},
 {"date":"2026-10-07","totals":{"cost":10},"tokenBreakdown":{"input":1000},"clients":[
  {"client":"claude","tokens":{"input":1,"output":2,"cacheRead":3,"cacheWrite":4,"reasoning":999},"cost":1},
  {"client":"codex","tokens":{"input":100},"cost":4},
  {"client":"zero","tokens":{},"cost":0},
  {"client":"tokens-only","tokens":{}}, {"client":"cost-only","cost":0},
  {"client":"unknown"}
 ]}
]}`
	var raw graph
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		t.Fatal(err)
	}
	got := convertGraph(raw, time.Date(2026, 10, 7, 12, 0, 0, 0, time.Local))
	for _, tc := range []struct {
		name    string
		period  usage.Period
		total   int64
		cost    float64
		clients map[string]int64
		costs   map[string]float64
	}{
		{"today", got.Today, 1000, 10, map[string]int64{"claude": 10, "codex": 100, "zero": 0, "tokens-only": 0, "partial": 0}, map[string]float64{"claude": 1, "codex": 4, "zero": 0, "cost-only": 0, "partial": 0}},
		{"month", got.Month, 3000, 30, map[string]int64{"claude": 30, "codex": 100, "zero": 0, "tokens-only": 0}, map[string]float64{"claude": 3, "codex": 4, "zero": 0, "cost-only": 0}},
		{"all", got.AllTime, 6000, 60, map[string]int64{"claude": 60, "codex": 100, "zero": 0, "tokens-only": 0}, map[string]float64{"claude": 6, "codex": 4, "zero": 0, "cost-only": 0}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.period.TotalTokens != tc.total || tc.period.CostUSD != tc.cost || tc.period.ClientBreakdown == nil || !reflect.DeepEqual(tc.period.Clients, tc.clients) || !reflect.DeepEqual(tc.period.ClientCosts, tc.costs) {
				t.Fatalf("period %+v, attribution %+v; want %v / %v", tc.period, tc.period.ClientBreakdown, tc.clients, tc.costs)
			}
		})
	}
}

func TestClientAttributionNeedsOnlyOneGraphCommand(t *testing.T) {
	f := newFakeTokscale(t)
	f.write("graph.json", `{"summary":{"clients":["claude","codex"]},"contributions":[{"date":"2026-10-07","clients":[{"client":"claude","tokens":{"input":12},"cost":1},{"client":"codex","tokens":{"output":34},"cost":2}]}]}`)
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	r := New(executable, filepath.Join(f.dir, "config"), f.scanFile(), testIntervals, usage.NewState(), nil)
	periods, clients, err := r.readPeriods(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if periods.AllTime.Clients["claude"] != 12 || periods.AllTime.Clients["codex"] != 34 || !reflect.DeepEqual(clients, []string{"claude", "codex"}) {
		t.Fatal("lost graph attribution")
	}
	log, err := os.ReadFile(filepath.Join(f.dir, "calls.log"))
	if err != nil {
		t.Fatal(err)
	}
	if len(f.runs("graph --no-spinner")) != 1 || bytes.Count(log, []byte("start|")) != 1 {
		t.Fatalf("extra command executions: %s", log)
	}
}

func TestConvertTokscalePeriods(t *testing.T) {
	const input = `{"contributions": [
  {"date": "2026-08-31", "totals": {"cost": 113.25}, "tokenBreakdown": {"input": 9000, "output": 18000, "cacheRead": 27000, "cacheWrite": 36000, "reasoning": 5}},
  {"date": "2026-09-01", "totals": {"cost": 11.25}, "tokenBreakdown": {"input": 900, "output": 1800, "cacheRead": 2700, "cacheWrite": 3600, "reasoning": 5}},
  {"date": "2026-09-28", "totals": {"cost": 1.25}, "tokenBreakdown": {"input": 101, "output": 202, "cacheRead": 303, "cacheWrite": 404, "reasoning": 5}}
]}`
	var raw graph
	if err := json.Unmarshal([]byte(input), &raw); err != nil {
		t.Fatal(err)
	}
	got := convertGraph(raw, time.Date(2026, 9, 28, 12, 0, 0, 0, time.Local))
	cases := []struct {
		name string
		got  usage.Period
		want int64
		cost float64
	}{
		{name: "today", got: got.Today, want: 1010, cost: 1.25},
		{name: "month", got: got.Month, want: 10010, cost: 12.5},
		{name: "all", got: got.AllTime, want: 100010, cost: 125.75},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if tc.got.TotalTokens != tc.want || tc.got.CostUSD != tc.cost {
				t.Fatalf("period = %+v, want tokens %d and cost %v", tc.got, tc.want, tc.cost)
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

func TestCountAgainJustAfterMidnight(t *testing.T) {
	cases := []struct{ now, want time.Time }{
		{time.Date(2026, 9, 30, 23, 59, 0, 0, time.Local), time.Date(2026, 10, 1, 0, 0, 1, 0, time.Local)},
		{time.Date(2026, 12, 31, 0, 0, 0, 0, time.Local), time.Date(2027, 1, 1, 0, 0, 1, 0, time.Local)},
	}
	for _, tc := range cases {
		if got := tc.now.Add(untilMidnight(tc.now)); !got.Equal(tc.want) {
			t.Fatalf("after %v, count again at %v, want %v", tc.now, got, tc.want)
		}
	}
}

func TestMergeLimitsKeepsProvidersThatTheReadLacks(t *testing.T) {
	percent := func(v float64) *float64 { return &v }
	claude := usage.Provider{Provider: "Claude", Windows: []usage.Window{{Kind: "session", ShowMeter: true, RemainingPercent: percent(83)}}}
	codex := usage.Provider{Provider: "Codex", Windows: []usage.Window{{Kind: "weekly", ShowMeter: true, RemainingPercent: percent(95)}}}
	first := usage.Limits{Providers: []usage.Provider{claude, codex}}
	if got := mergeLimits(nil, first); !reflect.DeepEqual(got, first) {
		t.Fatalf("first read = %+v", got)
	}
	// A read that skips Claude, for example by its rate limit, keeps Claude's last values.
	skipped := usage.Limits{Providers: []usage.Provider{{Provider: "Codex", Windows: []usage.Window{{Kind: "weekly", ShowMeter: true, RemainingPercent: percent(94)}}}}}
	kept := mergeLimits(&first, skipped)
	if len(kept.Providers) != 2 || kept.Providers[0].Provider != "Codex" || *kept.Providers[0].Windows[0].RemainingPercent != 94 ||
		kept.Providers[1].Provider != "Claude" || *kept.Providers[1].Windows[0].RemainingPercent != 83 {
		t.Fatalf("after a skipped provider = %+v", kept)
	}
	// When Claude is read again, its new values replace the kept ones.
	back := usage.Limits{Providers: []usage.Provider{{Provider: "Claude", Windows: []usage.Window{{Kind: "session", ShowMeter: true, RemainingPercent: percent(80)}}}, skipped.Providers[0]}}
	again := mergeLimits(&kept, back)
	if len(again.Providers) != 2 || *again.Providers[0].Windows[0].RemainingPercent != 80 {
		t.Fatalf("after the provider returned = %+v", again)
	}
}
