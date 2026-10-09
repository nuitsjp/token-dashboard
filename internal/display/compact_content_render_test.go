package display

import (
	"bytes"
	"image"
	"image/color"
	"testing"
	"time"

	"token-monitor-turzx/internal/usage"
)

// Match the literal, fully formatted value using the renderer's font rasterizer,
// independently of compactTokens/compactValue and their choice of font size.
func assertValueInk(t *testing.T, r *Renderer, img *image.RGBA, value string, bold bool, ink color.RGBA, right, baseline int, maxSize int) {
	t.Helper()
	for size := maxSize; size >= 3; size-- {
		face := r.face(bold, float64(size))
		x := right - measure(face, value)
		if x < 20 {
			continue
		}
		expected := image.NewRGBA(img.Bounds())
		fill(expected, expected.Bounds(), panelFill)
		r.text(expected, face, ink, x, baseline, value)
		matches, pixels := true, 0
		for y := max(0, baseline-maxSize-2); y < min(img.Bounds().Dy(), baseline+8); y++ {
			for xx := x; xx < right; xx++ {
				if expected.RGBAAt(xx, y) == panelFill {
					continue
				}
				pixels++
				if expected.RGBAAt(xx, y) != img.RGBAAt(xx, y) {
					matches = false
				}
			}
		}
		if matches && pixels > 0 {
			return
		}
	}
	t.Fatalf("full value %q absent at baseline %d", value, baseline)
}

func TestCompactSixServiceValuesAreFormattedInFull(t *testing.T) {
	r := testCompactRenderer(t)
	stats := &usage.Stats{Periods: usage.Periods{
		Today:   usage.Period{TotalTokens: 999, CostUSD: 999, ClientBreakdown: &usage.ClientBreakdown{Clients: map[string]int64{"tool": 1234567890123456789}, ClientCosts: map[string]float64{"tool": 123456789.12}}},
		Month:   usage.Period{ClientBreakdown: &usage.ClientBreakdown{Clients: map[string]int64{"tool": 2345678901234567890}, ClientCosts: map[string]float64{"tool": 234567890.23}}},
		AllTime: usage.Period{ClientBreakdown: &usage.ClientBreakdown{Clients: map[string]int64{"tool": 3456789012345678901}, ClientCosts: map[string]float64{"tool": 345678901.34}}},
	}, Limits: usage.Limits{Providers: []usage.Provider{{Provider: "tool", Windows: []usage.Window{compactWindow("5h", percent(70))}}}}}
	for _, style := range []Style{Gauges, Bars} {
		for _, orientation := range []string{"Landscape", "ReverseLandscape", "Portrait", "ReversePortrait"} {
			options := Options{Style: style, Orientation: orientation}
			pages, _ := compactPages(stats, options)
			img := r.renderCompact(stats, "Hub", options, &pages[0], 1, 1, "", time.Unix(0, 0))
			portrait := img.Bounds().Dx() == 320
			for i, tc := range []struct{ tokens, cost string }{{"1,234,567,890,123,456,789", "$123,456,789.12"}, {"2,345,678,901,234,567,890", "$234,567,890.23"}, {"3,456,789,012,345,678,901", "$345,678,901.34"}} {
				right, tokenY, costY, tokenSize, costSize := 198, 80+i*91, 106+i*91, 24, 18
				if portrait {
					right, tokenY, costY, tokenSize, costSize = 300, 74+i*48, 50+i*48, 20, 15
				}
				assertValueInk(t, r, img, tc.tokens, true, text, right, tokenY, tokenSize)
				assertValueInk(t, r, img, tc.cost, false, accent, right, costY, costSize)
			}
		}
	}
}

func TestCompactThreeGaugePositionsAndUnknownArcs(t *testing.T) {
	r := testCompactRenderer(t)
	stats := &usage.Stats{Limits: usage.Limits{Providers: []usage.Provider{{Provider: "tool", Windows: []usage.Window{compactWindow("A 5h", percent(10)), compactWindow("B 5h", percent(30)), compactWindow("C 5h", percent(80))}}}}}
	for _, tc := range []struct {
		orientation string
		tokens      bool
		cells       []image.Rectangle
	}{
		{"Landscape", true, []image.Rectangle{image.Rect(230, 80, 345, 181), image.Rect(345, 80, 460, 181), image.Rect(287, 181, 402, 282)}},
		{"Portrait", true, []image.Rectangle{image.Rect(20, 264, 160, 353), image.Rect(160, 264, 300, 353), image.Rect(90, 353, 230, 442)}},
		{"Landscape", false, []image.Rectangle{image.Rect(20, 80, 166, 282), image.Rect(166, 80, 313, 282), image.Rect(313, 80, 460, 282)}},
		{"Portrait", false, []image.Rectangle{image.Rect(20, 80, 300, 200), image.Rect(20, 200, 300, 321), image.Rect(20, 321, 300, 442)}},
	} {
		options := Options{Style: Gauges, Orientation: tc.orientation, ServiceContent: &serviceSelection{content: map[string]ServiceContent{"tool": {ShowLimits: true, ShowTokens: tc.tokens}}}}
		pages, _ := compactPages(stats, options)
		img := r.renderCompact(stats, "Local", options, &pages[0], 1, 1, "", time.Unix(0, 0))
		counts := []int{0, 0, 0}
		for y := 0; y < img.Bounds().Dy(); y++ {
			for x := 0; x < img.Bounds().Dx(); x++ {
				c := img.RGBAAt(x, y)
				for i, want := range []color.RGBA{gaugeBad, gaugeWarn, gaugeNormal} {
					if c == want {
						if !image.Pt(x, y).In(tc.cells[i]) {
							t.Fatalf("%s Tokens=%v circle %d escaped its cell: %d,%d", tc.orientation, tc.tokens, i, x, y)
						}
						counts[i]++
					}
				}
			}
		}
		for _, count := range counts {
			if count < 20 {
				t.Fatalf("circle missing: %v", counts)
			}
		}
	}
	for i := range stats.Limits.Providers[0].Windows {
		stats.Limits.Providers[0].Windows[i].RemainingPercent = nil
	}
	for _, style := range []Style{Gauges, Bars} {
		options := Options{Style: style}
		pages, _ := compactPages(stats, options)
		img := r.renderCompact(stats, "Local", options, &pages[0], 1, 1, "", time.Unix(0, 0))
		for y := 80; y < 282; y++ {
			for x := 230; x < 460; x++ {
				c := img.RGBAAt(x, y)
				if c == gaugeBad || c == gaugeWarn || c == gaugeNormal {
					t.Fatal("unknown remaining value drew a filled meter")
				}
			}
		}
	}
}

func TestCompactBarKindFallbackAndResetText(t *testing.T) {
	r := testCompactRenderer(t)
	now := time.Unix(0, 0)
	reset := now.Add(3*24*time.Hour + 4*time.Hour)
	stats := &usage.Stats{Limits: usage.Limits{Providers: []usage.Provider{{Provider: "tool", Windows: []usage.Window{{Kind: "weekly", WindowMinutes: percent(300), ShowMeter: true, RemainingPercent: percent(70), ResetsAt: &reset}}}}}}
	options := Options{Style: Bars}
	pages, _ := compactPages(stats, options)
	base := r.renderCompact(stats, "Local", options, &pages[0], 1, 1, "", now)
	stats.Limits.Providers[0].Windows[0].Label = "weekly"
	pages, _ = compactPages(stats, options)
	labeled := r.renderCompact(stats, "Local", options, &pages[0], 1, 1, "", now)
	if !bytes.Equal(base.Pix, labeled.Pix) {
		t.Fatal("empty Bars label did not fall back to kind")
	}
	assertValueInk(t, r, base, "3d 4h", false, dim, 460-measure(r.face(true, 20), "70%")-12, 122, 13)
}
