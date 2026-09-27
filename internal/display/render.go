// Package display draws the usage image shown on the TURZX and the preview.
package display

import (
	"cmp"
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"golang.org/x/image/font"
	"golang.org/x/image/font/opentype"
	"golang.org/x/image/math/fixed"

	"token-monitor-turzx/internal/usage"
)

const (
	Width  = 1920
	Height = 462
)

var (
	background = color.RGBA{0x0f, 0x11, 0x17, 0xff}
	divider    = color.RGBA{0x2a, 0x2f, 0x3a, 0xff}
	text       = color.RGBA{0xe8, 0xea, 0xf0, 0xff}
	dim        = color.RGBA{0x8a, 0x90, 0xa0, 0xff}
	accent     = color.RGBA{0x7c, 0xc4, 0xff, 0xff}
	track      = color.RGBA{0x2a, 0x2f, 0x3a, 0xff}
	good       = color.RGBA{0x4a, 0xde, 0x80, 0xff}
	warn       = color.RGBA{0xfb, 0xbf, 0x24, 0xff}
	bad        = color.RGBA{0xf8, 0x71, 0x71, 0xff}
)

// Renderer draws with Yu Gothic from the Windows font folder. It is not safe for concurrent use.
type Renderer struct {
	medium, bold *opentype.Font
	faces        map[faceKey]font.Face
}

type faceKey struct {
	bold bool
	size float64
}

func NewRenderer() (*Renderer, error) {
	dir := filepath.Join(os.Getenv("WINDIR"), "Fonts")
	medium, err := loadFont(filepath.Join(dir, "YuGothM.ttc"))
	if err != nil {
		return nil, err
	}
	bold, err := loadFont(filepath.Join(dir, "YuGothB.ttc"))
	if err != nil {
		return nil, err
	}
	return &Renderer{medium: medium, bold: bold, faces: map[faceKey]font.Face{}}, nil
}

// loadFont returns the first face of a collection: Yu Gothic Medium or Bold.
func loadFont(path string) (*opentype.Font, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read font: %w", err)
	}
	collection, err := opentype.ParseCollection(data)
	if err != nil {
		return nil, fmt.Errorf("parse font %s: %w", filepath.Base(path), err)
	}
	return collection.Font(0)
}

// Render draws stats as of now. Nil stats means no snapshot has arrived yet.
func (r *Renderer) Render(stats *usage.Stats, now time.Time) *image.RGBA {
	img := image.NewRGBA(image.Rect(0, 0, Width, Height))
	draw.Draw(img, img.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
	if stats == nil {
		face := r.face(false, 64)
		r.text(img, face, dim, (Width-measure(face, "Waiting for Hub"))/2, Height/2+22, "Waiting for Hub")
		return img
	}
	r.tokens(img, stats.Periods)
	r.limits(img, stats.Limits, now)
	return img
}

// tokensRight is the right edge of the Tokens column at the left of the image.
const tokensRight = 330

// tokens draws Today, Month and All from top to bottom, each as label and cost on one line
// with the tokens below. The tokens shrink until they fit the column.
func (r *Renderer) tokens(img *image.RGBA, periods usage.Periods) {
	blocks := []struct {
		label  string
		period usage.Period
	}{{"Today", periods.Today}, {"Month", periods.Month}, {"All", periods.AllTime}}
	label, cost := r.face(false, 20), r.face(false, 22)
	for i, b := range blocks {
		top := 30 + i*140
		r.text(img, label, dim, 40, top+28, b.label)
		c := usd(b.period.CostUSD)
		r.text(img, cost, accent, tokensRight-measure(cost, c), top+28, c)
		tokens := commas(fmt.Sprint(b.period.TotalTokens))
		size := 42.0
		for size > 20 && measure(r.face(true, size), tokens) > tokensRight-40 {
			size -= 2
		}
		r.text(img, r.face(true, size), text, 40, top+36+int(size), tokens)
	}
	fill(img, image.Rect(tokensRight+26, 30, tokensRight+28, Height-30), divider)
}

// group is one contract: the windows of a provider that show a meter, up to barRows.
type group struct {
	name, plan string
	windows    []usage.Window
}

func groups(limits usage.Limits) []group {
	var out []group
	for _, p := range limits.Providers {
		g := group{name: p.Provider, plan: p.PlanLabel}
		if g.plan == "" {
			g.plan = p.AccountLabel
		}
		for _, w := range p.Windows {
			if w.ShowMeter && len(g.windows) < barRows {
				g.windows = append(g.windows, w)
			}
		}
		if len(g.windows) > 0 {
			out = append(out, g)
		}
	}
	return out
}

// lowest is the smallest remaining percent of the group; a group without any reports above 100.
func (g group) lowest() float64 {
	low := math.Inf(1)
	for _, w := range g.windows {
		if w.RemainingPercent != nil {
			low = min(low, *w.RemainingPercent)
		}
	}
	return low
}

const (
	limitsLeft   = tokensRight + 54
	limitsTop    = 26
	columns      = 4
	columnGap    = 28
	columnWidth  = (Width - 40 - limitsLeft - (columns-1)*columnGap) / columns
	headerHeight = 54
	rowHeight    = 72
	barRows      = 4
	stackGap     = 18
	columnRows   = 4 // windows per column
	columnGroups = 2 // contracts per column
)

// layout sorts groups by their lowest remaining percent, keeping Hub order for ties, and puts
// each under the leftmost column that stays within columnRows windows and columnGroups contracts.
// A group that would need a fifth column is not shown.
func layout(gs []group) [][]group {
	sorted := slices.Clone(gs)
	slices.SortStableFunc(sorted, func(a, b group) int { return cmp.Compare(a.lowest(), b.lowest()) })
	var out [][]group
	rows := []int{}
	for _, g := range sorted {
		placed := false
		for i := range out {
			if len(out[i]) < columnGroups && rows[i]+len(g.windows) <= columnRows {
				out[i] = append(out[i], g)
				rows[i] += len(g.windows)
				placed = true
				break
			}
		}
		if !placed && len(out) < columns {
			out = append(out, []group{g})
			rows = append(rows, len(g.windows))
		}
	}
	return out
}

// limits draws the columns left to right, each contract with its name and plan on one line
// above its bars.
func (r *Renderer) limits(img *image.RGBA, limits usage.Limits, now time.Time) {
	name, plan := r.face(true, 36), r.face(false, 24)
	for i, column := range layout(groups(limits)) {
		x, y := limitsLeft+i*(columnWidth+columnGap), limitsTop
		for _, g := range column {
			n := truncate(name, g.name, columnWidth)
			r.text(img, name, text, x, y+33, n)
			if px := x + measure(name, n) + 14; px < x+columnWidth {
				r.text(img, plan, dim, px, y+33, truncate(plan, g.plan, x+columnWidth-px))
			}
			r.bars(img, g, x, y+headerHeight, now)
			y += headerHeight + len(g.windows)*rowHeight + stackGap
		}
	}
}

// bars draws each window as a line of label, time until reset and remaining percent, with a bar
// across the column below. The times until reset share one right edge so they line up.
func (r *Renderer) bars(img *image.RGBA, g group, x, y int, now time.Time) {
	big, label, small := r.face(true, 28), r.face(false, 21), r.face(false, 19)
	resetRight := x + columnWidth - measure(big, "100%") - 14
	for i, w := range g.windows {
		base := y + i*rowHeight + 25
		percent := "—"
		if w.RemainingPercent != nil {
			percent = fmt.Sprintf("%.0f%%", *w.RemainingPercent)
		}
		r.text(img, big, text, x+columnWidth-measure(big, percent), base, percent)
		reset := ""
		if w.ResetsAt != nil {
			reset = remaining(w.ResetsAt.Sub(now))
		}
		rw := measure(small, reset)
		r.text(img, small, dim, resetRight-rw, base, reset)
		name := w.Label
		if name == "" {
			name = w.Kind
		}
		r.text(img, label, text, x, base, truncate(label, name, resetRight-rw-10-x))
		fill(img, image.Rect(x, base+12, x+columnWidth, base+26), track)
		if w.RemainingPercent != nil {
			v := min(max(*w.RemainingPercent, 0), 100)
			fill(img, image.Rect(x, base+12, x+int(float64(columnWidth)*v/100), base+26), meterColor(v))
		}
	}
}

func (r *Renderer) face(bold bool, size float64) font.Face {
	key := faceKey{bold, size}
	if f, ok := r.faces[key]; ok {
		return f
	}
	src := r.medium
	if bold {
		src = r.bold
	}
	// NewFace fails only for invalid options, which are constants here.
	f, err := opentype.NewFace(src, &opentype.FaceOptions{Size: size, DPI: 72, Hinting: font.HintingFull})
	if err != nil {
		panic(err)
	}
	r.faces[key] = f
	return f
}

func (r *Renderer) text(img *image.RGBA, face font.Face, c color.Color, x, y int, s string) {
	d := font.Drawer{Dst: img, Src: image.NewUniform(c), Face: face, Dot: fixed.P(x, y)}
	d.DrawString(s)
}

func measure(face font.Face, s string) int { return font.MeasureString(face, s).Ceil() }

func truncate(face font.Face, s string, width int) string {
	if measure(face, s) <= width {
		return s
	}
	runes := []rune(s)
	for len(runes) > 0 && measure(face, string(runes)+"…") > width {
		runes = runes[:len(runes)-1]
	}
	return string(runes) + "…"
}

func fill(img *image.RGBA, rect image.Rectangle, c color.Color) {
	draw.Draw(img, rect, image.NewUniform(c), image.Point{}, draw.Src)
}

func meterColor(remaining float64) color.Color {
	switch {
	case remaining < 20:
		return bad
	case remaining < 50:
		return warn
	default:
		return good
	}
}

func usd(v float64) string {
	whole, cents, _ := strings.Cut(fmt.Sprintf("%.2f", v), ".")
	return "$" + commas(whole) + "." + cents
}

// commas groups the digits of a non-negative integer by thousands.
func commas(digits string) string {
	var b strings.Builder
	for i, c := range digits {
		if i > 0 && (len(digits)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return b.String()
}

// remaining formats the time until a reset as "2h 13m" below a day and "3d 4h" from a day.
func remaining(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	minutes := int(d / time.Minute)
	if minutes < 24*60 {
		return fmt.Sprintf("%dh %dm", minutes/60, minutes%60)
	}
	return fmt.Sprintf("%dd %dh", minutes/(24*60), minutes%(24*60)/60)
}
