package display

import (
	"image"
	"image/color"
	"reflect"
	"testing"
	"time"

	"token-monitor-turzx/internal/usage"
)

func TestFormats(t *testing.T) {
	for _, c := range []struct{ got, want string }{
		{commas("0"), "0"},
		{commas("999"), "999"},
		{commas("1000"), "1,000"},
		{commas("25984802353"), "25,984,802,353"},
		{usd(0), "$0.00"},
		{usd(14849.734), "$14,849.73"},
		{remaining(-time.Minute), "0h 0m"},
		{remaining(2*time.Hour + 13*time.Minute + 59*time.Second), "2h 13m"},
		{remaining(24*time.Hour - time.Second), "23h 59m"},
		{remaining(3*24*time.Hour + 4*time.Hour + 30*time.Minute), "3d 4h"},
	} {
		if c.got != c.want {
			t.Errorf("got %q, want %q", c.got, c.want)
		}
	}
}

func contract(name string, meters int) group {
	return group{name: name, windows: make([]usage.Window, meters)}
}

func names(columns [][]group) [][]string {
	out := [][]string{}
	for _, column := range columns {
		var c []string
		for _, g := range column {
			c = append(c, g.name)
		}
		out = append(out, c)
	}
	return out
}

func TestLayoutStacksSingleWindowContracts(t *testing.T) {
	got := names(layout([]group{
		contract("antigravity", 4), contract("claude", 2), contract("codex", 1), contract("cursor", 3),
		contract("grok", 1), contract("opencode", 3), contract("kimi", 1), contract("zai", 2),
	}))
	// codex and grok share a column, kimi does not fit under them, and zai finds no column left.
	want := [][]string{{"antigravity"}, {"claude"}, {"codex", "grok"}, {"cursor"}, {"opencode"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestGroupsKeepOnlyMeteredWindows(t *testing.T) {
	gs := groups(usage.Limits{Providers: []usage.Provider{
		{Provider: "a", AccountLabel: "Legacy", Windows: []usage.Window{{ShowMeter: true}, {ShowMeter: false}}},
		{Provider: "b", PlanLabel: "Pro", Windows: []usage.Window{{ShowMeter: false}}},
	}})
	if len(gs) != 1 || gs[0].name != "a" || gs[0].plan != "Legacy" || len(gs[0].windows) != 1 {
		t.Fatalf("got %+v", gs)
	}
}

func TestRotateClockwise(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 3, 2))
	top := color.RGBA{255, 0, 0, 255}
	src.SetRGBA(0, 0, top) // top-left goes to top-right
	dst := rotateClockwise(src)
	if dst.Bounds().Dx() != 2 || dst.Bounds().Dy() != 3 || dst.RGBAAt(1, 0) != top {
		t.Fatalf("size %v, top-right %v", dst.Bounds(), dst.RGBAAt(1, 0))
	}
}
