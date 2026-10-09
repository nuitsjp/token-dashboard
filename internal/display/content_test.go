package display

import (
	"errors"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"token-monitor-turzx/internal/usage"
)

func TestCompactContentServiceOrderAndPageChoices(t *testing.T) {
	stats := &usage.Stats{Periods: usage.Periods{Today: usage.Period{ClientBreakdown: &usage.ClientBreakdown{
		Clients: map[string]int64{"alpha": 0, "Alpha": 9, "z-token": 2}, ClientCosts: map[string]float64{"cost-only": 0},
	}}}, Limits: usage.Limits{Providers: []usage.Provider{
		{Provider: "beta", Windows: []usage.Window{compactWindow("5h", percent(70))}},
		{Provider: "alpha", AccountLabel: "one", Windows: []usage.Window{compactWindow("5h", percent(10))}},
		{Provider: "alpha", AccountLabel: "two", Windows: []usage.Window{compactWindow("5h", percent(20))}},
	}}}
	if got := serviceNames(stats, "Hub"); !reflect.DeepEqual(got, []string{"beta", "alpha", "Alpha", "cost-only", "z-token"}) {
		t.Fatalf("service IDs/order: %v", got)
	}
	selection := &serviceSelection{content: map[string]ServiceContent{
		"beta": {ShowLimits: false, ShowTokens: false}, "alpha": {ShowTokens: true},
		"Alpha": {ShowLimits: true}, "cost-only": {ShowTokens: true}, "z-token": {ShowLimits: false, ShowTokens: false},
	}}
	for _, style := range []Style{Gauges, Bars} {
		pages, message := compactPages(stats, Options{Style: style, ServiceContent: selection})
		if message != "" || len(pages) != 3 || pages[0].provider.Provider != "alpha" || pages[0].content.ShowLimits || pages[1].provider.Provider != "Alpha" || pages[1].message != "No usage limits to display" || pages[2].provider.Provider != "cost-only" {
			t.Fatalf("choices: %+v, %q", pages, message)
		}
	}
	// All Off excludes both limit and token-only services.
	selection.content["alpha"] = ServiceContent{}
	selection.content["Alpha"] = ServiceContent{}
	selection.content["cost-only"] = ServiceContent{}
	if pages, message := compactPages(stats, Options{ServiceContent: selection}); len(pages) != 0 || message != "No services selected" {
		t.Fatalf("all Off: %d %q", len(pages), message)
	}
	if got := contentOf(nil, "new"); !got.ShowLimits || !got.ShowTokens {
		t.Fatal("new service is not Both")
	}
}

func TestCompactThreeCirclesKeepTwoWindowsPerGroup(t *testing.T) {
	for _, tc := range []struct {
		name         string
		windows      []usage.Window
		gauges, bars []int
	}{
		{"same group three", []usage.Window{{Kind: "billing", Label: "Shared Monthly", ShowMeter: true}, {Kind: "weekly", Label: "Shared WEEKLY", ShowMeter: true}, {Kind: "session", Label: "Shared session", ShowMeter: true}}, []int{2}, []int{3}},
		{"independent three", []usage.Window{compactWindow("A 5h", nil), compactWindow("B 5h", nil), compactWindow("C 5h", nil)}, []int{3}, []int{3}},
		{"paired three", []usage.Window{compactWindow("A 5h", nil), {Kind: "weekly", Label: "A weekly", ShowMeter: true}, compactWindow("B 5h", nil), {Kind: "weekly", Label: "B weekly", ShowMeter: true}, compactWindow("C 5h", nil), {Kind: "weekly", Label: "C weekly", ShowMeter: true}}, []int{3}, []int{3, 3}},
		{"four circles", []usage.Window{compactWindow("A 5h", nil), compactWindow("B 5h", nil), compactWindow("C 5h", nil), compactWindow("D 5h", nil)}, []int{3, 1}, []int{3, 1}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stats := &usage.Stats{Limits: usage.Limits{Providers: []usage.Provider{{Provider: "tool", Windows: tc.windows}}}}
			for _, style := range []Style{Gauges, Bars} {
				pages, _ := compactPages(stats, Options{Style: style})
				var counts []int
				windows := 0
				for _, page := range pages {
					if style == Bars {
						counts = append(counts, len(page.windows))
						windows += len(page.windows)
						continue
					}
					counts = append(counts, len(page.panel.circles))
					for _, circle := range page.panel.circles {
						if len(circle.windows) > 2 {
							t.Fatal("more than two rings")
						}
						for i, w := range circle.windows {
							windows++
							if groupOf(w) != groupOf(circle.windows[0]) {
								t.Fatal("mixed groups")
							}
							if i > 0 {
								outer, _ := lengthOf(circle.windows[i-1])
								inner, _ := lengthOf(w)
								if outer > inner {
									t.Fatal("long window outside")
								}
							}
						}
					}
				}
				want := tc.gauges
				if style == Bars {
					want = tc.bars
				}
				if !reflect.DeepEqual(counts, want) || windows != len(tc.windows) {
					t.Fatalf("%s pages %v, windows %d; want %v/%d", style, counts, windows, want, len(tc.windows))
				}
			}
		})
	}
}

func TestTokensOnlyStillSkipsFullServiceAndRetainsUnknown(t *testing.T) {
	selection := &serviceSelection{content: map[string]ServiceContent{"tool": {ShowTokens: true}}}
	options := Options{ServiceContent: selection, SkipFull5hServices: true}
	stats := &usage.Stats{Limits: usage.Limits{Providers: []usage.Provider{{Provider: "tool", Windows: []usage.Window{compactWindow("5h", percent(100))}}}}}
	if pages, message := compactPages(stats, options); len(pages) != 0 || message != "All services skipped" {
		t.Fatalf("full Tokens service shown: %d %q", len(pages), message)
	}
	for _, value := range []*float64{nil, percent(99.999)} {
		stats.Limits.Providers[0].Windows[0].RemainingPercent = value
		if pages, _ := compactPages(stats, options); len(pages) != 1 || pages[0].content.ShowLimits {
			t.Fatal("unknown/rounded Tokens service skipped")
		}
	}
}

func TestSelectionFailureAndDataUpdatesPreserveRotation(t *testing.T) {
	state := usage.NewState()
	state.Set(&usage.Stats{Limits: usage.Limits{Providers: []usage.Provider{{Provider: "alpha"}, {Provider: "beta"}}}})
	content := map[string]ServiceContent{"alpha": {ShowLimits: true, ShowTokens: true}}
	s := &Service{State: state, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)), Content: func() (map[string]ServiceContent, error) { return content, nil }}
	first, err := Selection(s)
	if err != nil {
		t.Fatal(err)
	}
	pages, _ := compactPages(state.Latest(), Options{ServiceContent: first})
	options := Options{Compact: true, ServiceContent: first, Interval: 10 * time.Second}
	now := time.Unix(1000, 0)
	var cycle rotation
	cycle.update(pages, options, now)
	cycle.update(pages, options, now.Add(10*time.Second))
	deadline := cycle.deadline
	state.Touch()
	again, _ := Selection(s)
	if again != first {
		t.Fatal("data redraw changed selection identity")
	}
	s.SaveContent = func(string, bool, bool, bool) error { return errors.New("disk full") }
	if _, err := s.SetServiceContent("alpha", false, true, true); err == nil {
		t.Fatal("failed save succeeded")
	}
	after, _ := Selection(s)
	if after != first {
		t.Fatal("failed save published selection")
	}
	options.ServiceContent = after
	cycle.update(pages, options, now.Add(11*time.Second))
	if cycle.index != 1 || !cycle.deadline.Equal(deadline) {
		t.Fatal("failed save/data update reset rotation")
	}
	content["alpha"] = ServiceContent{ShowTokens: true}
	changed, _ := Selection(s)
	if changed == first || !first.content["alpha"].ShowLimits {
		t.Fatal("selection was mutated instead of replaced")
	}
	options.ServiceContent = changed
	cycle.update(pages, options, now.Add(12*time.Second))
	if cycle.index != 0 || !cycle.deadline.Equal(now.Add(22*time.Second)) {
		t.Fatal("content change did not restart rotation")
	}
}

func TestEnabledSelectionReloadsAndFailuresKeepRotation(t *testing.T) {
	state := usage.NewState()
	state.Set(&usage.Stats{Limits: usage.Limits{Providers: []usage.Provider{{Provider: "alpha"}, {Provider: "beta"}}}})
	<-state.Changed()
	enabled := true
	saved := ServiceContent{Provider: "alpha", Enabled: &enabled, ShowLimits: true}
	failSave := false
	s := &Service{State: state, Logger: slog.New(slog.NewTextHandler(io.Discard, nil)),
		Content: func() (map[string]ServiceContent, error) {
			// Reading JSON allocates new bool pointers even when its values are unchanged.
			copy := saved
			enabled := *saved.Enabled
			copy.Enabled = &enabled
			return map[string]ServiceContent{"alpha": copy}, nil
		},
		SaveContent: func(provider string, enabled, limits, tokens bool) error {
			if failSave {
				return errors.New("disk full")
			}
			saved = ServiceContent{Provider: provider, Enabled: &enabled, ShowLimits: limits, ShowTokens: tokens}
			return nil
		},
	}
	first, err := Selection(s)
	if err != nil {
		t.Fatal(err)
	}
	options := Options{Compact: true, ServiceContent: first, Interval: 10 * time.Second}
	pages, _ := compactPages(state.Latest(), options)
	now := time.Unix(1000, 0)
	var cycle rotation
	cycle.update(pages, options, now)
	cycle.update(pages, options, now.Add(10*time.Second))
	deadline := cycle.deadline
	for i := range 3 {
		reloaded, err := Selection(s)
		if err != nil || reloaded != first {
			t.Fatalf("equal saved values replaced the selection: %v", err)
		}
		options.ServiceContent = reloaded
		cycle.update(pages, options, now.Add(time.Duration(11+i)*time.Second))
		if cycle.index != 1 || !cycle.deadline.Equal(deadline) {
			t.Fatal("rereading equal enabled values reset rotation")
		}
	}
	if _, err := s.SetServiceContent("alpha", false, true, false); err != nil {
		t.Fatal(err)
	}
	select {
	case <-state.Changed():
	default:
		t.Fatal("successful service toggle did not request redraw")
	}
	changed, _ := Selection(s)
	if changed == first || !*contentOf(first, "alpha").Enabled || *contentOf(changed, "alpha").Enabled {
		t.Fatal("toggle did not replace an immutable selection")
	}
	options.ServiceContent = changed
	pages, _ = compactPages(state.Latest(), options)
	cycle.update(pages, options, now.Add(14*time.Second))
	if len(pages) != 1 || pages[0].provider.Provider != "beta" || cycle.index != 0 || !cycle.deadline.Equal(now.Add(24*time.Second)) {
		t.Fatal("OFF toggle did not exclude the service and restart rotation")
	}
	failSave = true
	for _, choice := range []struct{ enabled, limits, tokens bool }{{true, true, false}, {false, false, true}} {
		if _, err := s.SetServiceContent("alpha", choice.enabled, choice.limits, choice.tokens); err == nil {
			t.Fatal("failed save succeeded")
		}
		after, _ := Selection(s)
		if after != changed || *saved.Enabled || !saved.ShowLimits || saved.ShowTokens {
			t.Fatal("failed save changed enabled/content state")
		}
		select {
		case <-state.Changed():
			t.Fatal("failed save requested redraw")
		default:
		}
		options.ServiceContent = after
		cycle.update(pages, options, now.Add(15*time.Second))
		if cycle.index != 0 || !cycle.deadline.Equal(now.Add(24*time.Second)) {
			t.Fatal("failed save changed rotation")
		}
	}
}
