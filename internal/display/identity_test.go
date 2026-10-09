package display

import (
	"bytes"
	"context"
	"encoding/json"
	"image"
	"io"
	"log/slog"
	"reflect"
	"testing"
	"time"

	"token-monitor-turzx/internal/usage"
)

func TestLocalAliasesKeepRawWindowKeysAndReportedStats(t *testing.T) {
	stats := &usage.Stats{Limits: usage.Limits{Providers: []usage.Provider{
		{Provider: "Codex", AccountLabel: "first", Windows: []usage.Window{compactWindow("5h", percent(20))}},
		{Provider: "codex", AccountLabel: "second", Windows: []usage.Window{compactWindow("5h", percent(10))}},
		{Provider: "Claude", Windows: []usage.Window{compactWindow("5h", percent(80))}},
	}}}
	before, _ := json.Marshal(stats)
	for _, style := range []Style{Gauges, Bars} {
		options := Options{source: "Local", Style: style}
		pages, _ := compactPages(stats, options)
		if len(pages) != 3 || pages[0].provider.Provider != "Codex" || pages[0].provider.AccountLabel != "second" || pages[1].provider.AccountLabel != "first" || pages[2].provider.Provider != "Claude" {
			t.Fatalf("merged service/contract order: %+v", pages)
		}
		p := stats.Limits.Providers[1]
		if pages[0].key != compactKey(p, p.Windows) {
			t.Fatal("canonical service name changed the original page/window identity")
		}
		hidden := map[string]bool{windowKey(p, p.Windows[0]): true}
		visible := withoutHidden(stats, hidden)
		pages, _ = compactPages(visible, options)
		if len(pages) != 2 || pages[0].provider.AccountLabel != "first" {
			t.Fatal("saved raw window key no longer hides its original contract")
		}
	}
	after, _ := json.Marshal(stats)
	if !bytes.Equal(before, after) {
		t.Fatal("compact mapping mutated the reported usage data")
	}
}

func TestLocalMergedTokensSkipAndRotationUseAllVisibleContracts(t *testing.T) {
	stats := &usage.Stats{Limits: usage.Limits{Providers: []usage.Provider{
		{Provider: "Codex", AccountLabel: "one", Windows: []usage.Window{compactWindow("5h", percent(100))}},
		{Provider: "codex", AccountLabel: "two", Windows: []usage.Window{compactWindow("5h", percent(99.999))}},
		{Provider: "Claude", Windows: []usage.Window{compactWindow("weekly", percent(50))}},
	}}}
	options := Options{source: "Local", Compact: true, SkipFull5hServices: true, Interval: 10 * time.Second,
		ServiceContent: &serviceSelection{content: map[string]ServiceContent{"Codex": {ShowTokens: true}, "Claude": {ShowTokens: true}}}}
	pages, _ := compactPages(stats, options)
	if len(pages) != 2 || pages[0].content.ShowLimits {
		t.Fatal("rounded 100% skipped merged Tokens-only service or repeated its contracts")
	}
	var cycle rotation
	now := time.Unix(1000, 0)
	cycle.update(pages, options, now)
	deadline := cycle.deadline
	stats.Periods.Today = usage.Period{ClientBreakdown: &usage.ClientBreakdown{Clients: map[string]int64{"codex": 123}}}
	stats.Limits.Providers[1].Windows[0].RemainingPercent = nil
	pages, _ = compactPages(stats, options)
	cycle.update(pages, options, now.Add(3*time.Second))
	if len(pages) != 2 || cycle.index != 0 || !cycle.deadline.Equal(deadline) {
		t.Fatal("unknown remaining value or Tokens update reset/removed the current page")
	}
	p := stats.Limits.Providers[1]
	visible := withoutHidden(stats, map[string]bool{windowKey(p, p.Windows[0]): true})
	pages, _ = compactPages(visible, options)
	cycle.update(pages, options, now.Add(4*time.Second))
	if len(pages) != 1 || pages[0].provider.Provider != "Claude" || !cycle.deadline.Equal(deadline) {
		t.Fatal("hiding unknown 5h should skip full Codex and advance without restarting the deadline")
	}
	stats.Limits.Providers[1].Windows[0].RemainingPercent = percent(100)
	if full, _ := compactPages(stats, options); len(full) != 1 || full[0].provider.Provider != "Claude" {
		t.Fatal("full 5h across both aliases did not skip the whole service")
	}
	if got := serviceContent(options.ServiceContent, "Local", "Codex"); !*got.Enabled || !got.ShowTokens {
		t.Fatal("skip changed the saved service choice")
	}
}

func TestWideRunIgnoresCompactAliasesAndServiceChoices(t *testing.T) {
	stats := &usage.Stats{Periods: usage.Periods{Today: usage.Period{TotalTokens: 54321, CostUSD: 4.56,
		ClientBreakdown: &usage.ClientBreakdown{Clients: map[string]int64{"codex": 123}}}},
		Limits: usage.Limits{Providers: []usage.Provider{
			{Provider: "Codex", Windows: []usage.Window{compactWindow("5h", percent(20))}},
			{Provider: "codex", Windows: []usage.Window{compactWindow("weekly", percent(50))}},
		}}}
	for _, style := range []Style{Gauges, Bars} {
		var reference *image.RGBA
		for _, source := range []string{"Hub", "Local"} {
			r := testCompactRenderer(t)
			state := usage.NewState()
			state.SetSource(source)
			state.Set(stats)
			logger := slog.New(slog.NewTextHandler(io.Discard, nil))
			options := Options{Style: style, ServiceContent: &serviceSelection{content: map[string]ServiceContent{"Codex": {}, "codex": {}}}}
			s := &Service{State: state, Hidden: func() ([]string, error) { return nil, nil }, Options: func() (Options, error) { return options, nil }}
			ctx, cancel := context.WithCancel(context.Background())
			frames := make(chan Frame, 2)
			done := make(chan struct{})
			go func() {
				defer close(done)
				Run(ctx, s, r, state, time.Minute, func() Style { return style }, func(f Frame) { frames <- f }, func(string, any) {}, logger)
			}()
			var frame Frame
			select {
			case frame = <-frames:
			case <-time.After(3 * time.Second):
				cancel()
				<-done
				t.Fatal("wide frame not produced")
			}
			cancel()
			<-done
			if frame.Target.Compact || frame.Image.Bounds() != image.Rect(0, 0, Width, Height) || !reflect.DeepEqual(frame.Target, options) {
				t.Fatal("compact settings changed the wide frame destination")
			}
			if reference != nil && !bytes.Equal(reference.Pix, frame.Image.Pix) {
				t.Fatal("Local aliases or compact OFF choices changed the 9.2-inch image")
			}
			reference = frame.Image
		}
	}
}
