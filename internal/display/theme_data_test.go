package display

import (
	"reflect"
	"testing"
	"time"

	"token-monitor-turzx/internal/usage"
)

func themeProvider(name string, values ...float64) usage.Provider {
	p := usage.Provider{Provider: name, AccountLabel: "Account"}
	for _, value := range values {
		w := usage.Window{Kind: "daily", ShowMeter: true}
		if value >= 0 {
			w.RemainingPercent = &value
		}
		p.Windows = append(p.Windows, w)
	}
	return p
}

// Nil snapshots must display the selected source's waiting message, without invented statistics.
func TestThemeDataWaiting(t *testing.T) {
	for _, source := range []string{"Hub", "Local"} {
		data := themeData(nil, time.Now(), source)
		want := "Waiting for Hub"
		if source == "Local" {
			want = "Waiting for local usage"
		}
		if data.Available || data.WaitingMessage != want || len(data.Tokens)+len(data.GaugePanels)+len(data.BarColumns) != 0 {
			t.Fatalf("%s: %+v", source, data)
		}
	}
}

// Sorting and packing must keep the most depleted contracts and preserve source order for ties.
func TestThemeDataOrderAndCapacity(t *testing.T) {
	stats := &usage.Stats{Limits: usage.Limits{Providers: []usage.Provider{
		themeProvider("antigravity", 100, 99, 100, 100), themeProvider("claude", 48, 82), themeProvider("codex", 87),
		themeProvider("cursor", 95, 71, 100), themeProvider("unknown", -1), themeProvider("opencode", 100, 97, 89),
		themeProvider("copilot", 42), themeProvider("grok", 87),
	}}}
	data := themeData(stats, time.Now(), "Hub")
	var barNames [][]string
	for _, column := range data.BarColumns {
		var names []string
		rows := 0
		for _, contract := range column.Contracts {
			names = append(names, contract.Provider)
			rows += len(contract.Windows)
		}
		if rows > 4 || len(column.Contracts) > 2 {
			t.Fatalf("overfull column: %+v", column)
		}
		barNames = append(barNames, names)
	}
	wantBars := [][]string{{"copilot", "claude"}, {"cursor", "codex"}, {"grok", "opencode"}, {"antigravity"}}
	if !reflect.DeepEqual(barNames, wantBars) {
		t.Fatalf("bars = %v, want %v", barNames, wantBars)
	}
	var panelNames []string
	for _, panel := range data.GaugePanels {
		panelNames = append(panelNames, panel.Provider)
	}
	wantPanels := []string{"copilot", "claude", "cursor", "codex", "grok", "unknown"}
	if !reflect.DeepEqual(panelNames, wantPanels) {
		t.Fatalf("gauges = %v, want %v", panelNames, wantPanels)
	}
}

// Hiding the lowest window must change both layouts without modifying the shared snapshot.
func TestThemeDataAfterHiddenSelection(t *testing.T) {
	stats := &usage.Stats{Limits: usage.Limits{Providers: []usage.Provider{
		themeProvider("claude", 10, 80), themeProvider("codex", 40),
	}}}
	stats.Limits.Providers[0].Windows[0].Label = "Session"
	stats.Limits.Providers[0].Windows[1].Label = "Weekly"
	hidden := map[string]bool{windowKey(stats.Limits.Providers[0], stats.Limits.Providers[0].Windows[0]): true}
	data := themeData(withoutHidden(stats, hidden), time.Now(), "Hub")
	if data.GaugePanels[0].Provider != "codex" || data.BarColumns[0].Contracts[0].Provider != "codex" {
		t.Fatalf("hidden lowest value still affects order: %+v", data)
	}
	claude := data.GaugePanels[1]
	if len(claude.Circles) != 1 || claude.Circles[0].Double || claude.Circles[0].Windows[0].Label != "Weekly" {
		t.Fatalf("hidden window still drawn: %+v", claude)
	}
	if len(stats.Limits.Providers[0].Windows) != 2 {
		t.Fatal("shared snapshot was mutated")
	}
}

// A group pairs shorter and longer windows, while distinct model groups retain their names.
func TestThemeDataDoubleWindowsAndGroups(t *testing.T) {
	p := themeProvider("Claude", 70, 60, 50, 1)
	p.PlanLabel = "Max"
	p.Windows[0].Kind, p.Windows[0].Label = "weekly", "Model A Weekly"
	p.Windows[1].Kind, p.Windows[1].Label = "session", "Model A Session"
	p.Windows[2].Kind, p.Windows[2].Label = "daily", "Model B Daily"
	p.Windows[3].ShowMeter = false
	data := themeData(&usage.Stats{Limits: usage.Limits{Providers: []usage.Provider{p}}}, time.Now(), "Hub")
	panel := data.GaugePanels[0]
	if panel.Span != 2 || panel.Plan != "Max" || panel.Icon != "/assets/agents/claude.png" {
		t.Fatalf("panel: %+v", panel)
	}
	a, b := panel.Circles[0], panel.Circles[1]
	if a.Name != "Model A" || !a.Double || !a.ShowName || a.Windows[0].DurationLabel != "5h" || a.Windows[1].DurationLabel != "7d" {
		t.Fatalf("paired windows: %+v", a)
	}
	if b.Name != "Model B" || b.Double || !b.ShowName || len(data.BarColumns[0].Contracts[0].Windows) != 3 {
		t.Fatalf("distinct group or unmetered window: %+v", data)
	}
}

// Missing reports remain unknown, and pace warnings must survive conversion into CSS tones.
func TestThemeDataUnknownAndPace(t *testing.T) {
	now := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	p := themeProvider("unknown-provider", -1, 60, 20)
	reset := now.Add(24 * time.Hour)
	p.Windows[1].ResetsAt = &reset
	data := themeData(&usage.Stats{
		Periods: usage.Periods{Today: usage.Period{TotalTokens: 1234567, CostUSD: 12.34}},
		Limits:  usage.Limits{Providers: []usage.Provider{p}},
	}, now, "Hub")
	windows := data.BarColumns[0].Contracts[0].Windows
	if windows[0].HasRemaining || windows[0].RemainingPercent != nil || windows[0].PercentText != "—" || windows[0].ResetText != "—" || windows[0].BarResetText != "" || windows[0].Label != "daily" {
		t.Fatalf("unknown report: %+v", windows[0])
	}
	if windows[1].Tone != "warning" || windows[1].PercentText != "60%" || windows[1].ResetText != "1d 0h" || windows[2].Tone != "danger" {
		t.Fatalf("pace/remaining state: %+v", windows)
	}
	if data.GaugePanels[0].Icon != "" || data.GaugePanels[0].Circles[0].ShowName {
		t.Fatalf("unknown icon/single group: %+v", data.GaugePanels[0])
	}
	if data.Tokens[0].TokensText != "1,234,567" || data.Tokens[0].CostText != "$12.34" || data.Tokens[0].TokensFontSize != 42 {
		t.Fatalf("tokens: %+v", data.Tokens)
	}
}

// The two styles retain their existing rounding, while SVG fills stay in the track.
func TestThemeDataRoundingAndFillBounds(t *testing.T) {
	p := themeProvider("claude", 72.5, 120, -1)
	data := themeData(&usage.Stats{Limits: usage.Limits{Providers: []usage.Provider{p}}}, time.Now(), "Hub")
	if got := data.GaugePanels[0].Circles[0].Windows[0].PercentText; got != "73%" {
		t.Fatalf("gauge rounding: %s", got)
	}
	if got := data.BarColumns[0].Contracts[0].Windows[0].PercentText; got != "72%" {
		t.Fatalf("bar rounding: %s", got)
	}
	if got := *data.BarColumns[0].Contracts[0].Windows[1].RemainingPercent; got != 100 {
		t.Fatalf("SVG fill outside its track: %v", got)
	}
	if got := *p.Windows[1].RemainingPercent; got != 120 {
		t.Fatalf("shared report changed: %v", got)
	}
}
