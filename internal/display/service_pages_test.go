package display

import (
	"reflect"
	"testing"
	"time"

	"token-monitor-turzx/internal/usage"
)

func serviceConfig() Configuration {
	return Configuration{Source: "Hub", Theme: Theme{ID: "small", Width: 480, Height: 320, DataLayout: "service-gauges"}}
}

func quota(kind, label string, remaining *float64) usage.Window {
	return usage.Window{Kind: kind, Label: label, ShowMeter: true, RemainingPercent: remaining}
}

func number(value float64) *float64 { return &value }

func TestServiceWaitingEmptyAndDisabledStatesAreDistinct(t *testing.T) {
	config := serviceConfig()
	config.Presentation.SkipFull5h = true
	for _, tc := range []struct {
		stats   *usage.Stats
		message string
	}{
		{nil, "Waiting for Hub"},
		{&usage.Stats{}, "No services to display"},
	} {
		pages, message := ServicePages(tc.stats, time.Now(), config, nil)
		if len(pages) != 0 || message != tc.message {
			t.Fatalf("empty state: %d pages, %q", len(pages), message)
		}
	}
	config.Presentation.Services = map[string]map[string]ServiceContent{"Hub": {"A": {Enabled: false, Content: "Limits"}}}
	pages, message := ServicePages(scheduledServices("A"), time.Now(), config, nil)
	if len(pages) != 0 || message != "No services selected" {
		t.Fatalf("disabled state: %d pages, %q", len(pages), message)
	}
}

func TestServiceSkipUsesVisibleLimitsAndExactFiveHourValues(t *testing.T) {
	for _, content := range []string{"Both", "Limits", "Tokens"} {
		for _, tc := range []struct {
			name    string
			windows []usage.Window
			extra   []usage.Window
			hidden  bool
			skipped bool
		}{
			{name: "not reported", skipped: true},
			{name: "not metered", windows: []usage.Window{{Kind: "session", RemainingPercent: number(40)}}, skipped: true},
			{name: "last window hidden", windows: []usage.Window{quota("session", "5h", number(40))}, hidden: true, skipped: true},
			{name: "exactly full", windows: []usage.Window{quota("session", "5h", number(100))}, skipped: true},
			{name: "rounds to full", windows: []usage.Window{quota("session", "5h", number(99.999))}},
			{name: "unknown remaining", windows: []usage.Window{quota("session", "5h", nil)}},
			{name: "weekly only", windows: []usage.Window{quota("weekly", "Weekly", number(100))}},
			{name: "explicit duration overrides kind", windows: []usage.Window{{Kind: "session", ShowMeter: true, RemainingPercent: number(100), WindowMinutes: number(1440)}}},
			{name: "explicit five hours", windows: []usage.Window{{Kind: "weekly", ShowMeter: true, RemainingPercent: number(100), WindowMinutes: number(300)}}, skipped: true},
			{name: "other contract used", windows: []usage.Window{quota("session", "5h", number(100))}, extra: []usage.Window{quota("session", "5h", number(20))}},
			{name: "other contract unknown", windows: []usage.Window{quota("session", "5h", number(100))}, extra: []usage.Window{quota("session", "5h", nil)}},
		} {
			t.Run(content+"/"+tc.name, func(t *testing.T) {
				p := usage.Provider{Provider: "Copilot", AccountLabel: "first", Windows: tc.windows}
				stats := &usage.Stats{Periods: usage.Periods{Today: usage.Period{ClientBreakdown: &usage.ClientBreakdown{Clients: map[string]int64{"Copilot": 0}}}}, Limits: usage.Limits{Providers: []usage.Provider{p}}}
				if tc.extra != nil {
					stats.Limits.Providers = append(stats.Limits.Providers, usage.Provider{Provider: "Copilot", AccountLabel: "second", Windows: tc.extra})
				}
				config := serviceConfig()
				config.Presentation = Presentation{SkipFull5h: true, Services: map[string]map[string]ServiceContent{"Hub": {"Copilot": {Enabled: true, Content: content}}}}
				hidden := map[string]bool{}
				if tc.hidden {
					hidden[windowKey(p, p.Windows[0])] = true
				}
				pages, message := ServicePages(stats, time.Now(), config, hidden)
				if tc.skipped {
					if len(pages) != 0 || message != "All services skipped" {
						t.Fatalf("skipped service: pages=%d message=%q", len(pages), message)
					}
				} else if len(pages) == 0 || message != "" {
					t.Fatalf("visible service: pages=%d message=%q", len(pages), message)
				}
				config.Presentation.SkipFull5h = false
				pages, _ = ServicePages(stats, time.Now(), config, hidden)
				if len(pages) == 0 || pages[0].Provider != "Copilot" {
					t.Fatal("OFF did not restore the service")
				}
			})
		}
	}
}

func TestServiceTokensKeepMissingZeroAndExactSourceIdentities(t *testing.T) {
	stats := &usage.Stats{Periods: usage.Periods{Today: usage.Period{TotalTokens: 999, CostUSD: 999, ClientBreakdown: &usage.ClientBreakdown{
		Clients: map[string]int64{"claude": 0, "antigravity-cli": 12, "opencode": 13}, ClientCosts: map[string]float64{"codex": 0},
	}}}, Limits: usage.Limits{Providers: []usage.Provider{{Provider: "Claude"}, {Provider: "Codex"}, {Provider: "Antigravity"}, {Provider: "OpenCode Go"}}}}
	config := serviceConfig()
	config.Source = "Local"
	pages, _ := ServicePages(stats, time.Now(), config, nil)
	byName := map[string]ServicePage{}
	for _, page := range pages {
		byName[page.Provider] = page
	}
	if len(byName) != 6 || byName["Claude"].Tokens[0].TokensText != "0" || byName["Claude"].Tokens[0].CostText != "—" || byName["Codex"].Tokens[0].TokensText != "—" || byName["Codex"].Tokens[0].CostText != "$0.00" {
		t.Fatalf("missing attribution or zero was changed: %+v", byName)
	}
	if byName["Antigravity"].Tokens[0].TokensText != "—" || byName["antigravity-cli"].Tokens[0].TokensText != "12" || byName["OpenCode Go"].Tokens[0].TokensText != "—" {
		t.Fatal("unrelated service identities were merged")
	}
	config.Source = "Hub"
	pages, _ = ServicePages(stats, time.Now(), config, nil)
	if len(pages) != 8 {
		t.Fatalf("Hub identifiers were normalized: %d pages", len(pages))
	}
}

func TestServicePagesDoNotRepeatTokensOrDropOverflow(t *testing.T) {
	stats := &usage.Stats{Limits: usage.Limits{Providers: []usage.Provider{
		{Provider: "Claude", AccountLabel: "high", Windows: []usage.Window{quota("session", "Alpha 5h", number(80))}},
		{Provider: "Claude", AccountLabel: "low", Windows: []usage.Window{quota("session", "Alpha 5h", number(10)), quota("weekly", "Alpha Weekly", number(40)), quota("session", "Beta 5h", number(30)), quota("session", "Gamma 5h", number(50)), quota("session", "Delta 5h", number(60))}},
	}}}
	config := serviceConfig()
	for _, layout := range []string{"service-gauges", "service-bars"} {
		config.Theme.DataLayout = layout
		pages, _ := ServicePages(stats, time.Now(), config, nil)
		if len(pages) != 3 || pages[0].Plan != "low" || pages[2].Plan != "high" {
			t.Fatalf("%s contract order/capacity: %+v", layout, pages)
		}
		var labels []string
		for _, page := range pages {
			if len(page.Circles) > 3 || layout == "service-bars" && len(page.Windows) > 3 {
				t.Fatal("page exceeds style capacity")
			}
			for _, w := range page.Windows {
				labels = append(labels, w.ID)
			}
		}
		if len(labels) != 6 || pages[0].Number != 1 || pages[2].Number != 3 || pages[0].Count != 3 {
			t.Fatalf("overflow or page numbers: %v", labels)
		}
	}
	config.Presentation.Services = map[string]map[string]ServiceContent{"Hub": {"Claude": {Enabled: true, Content: "Tokens"}}}
	pages, _ := ServicePages(stats, time.Now(), config, nil)
	if len(pages) != 1 || !pages[0].ShowTokens || pages[0].ShowLimits {
		t.Fatal("service-wide tokens were repeated per contract")
	}
	before := ServiceChoices(stats, "Hub", config.Presentation)
	config.Presentation.SkipFull5h = true
	_, _ = ServicePages(stats, time.Now(), config, map[string]bool{windowKey(stats.Limits.Providers[0], stats.Limits.Providers[0].Windows[0]): true})
	if !reflect.DeepEqual(before, ServiceChoices(stats, "Hub", config.Presentation)) {
		t.Fatal("page selection mutated service preferences")
	}
}
