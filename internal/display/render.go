// Package display draws the usage image shown on the TURZX and the preview.
package display

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"os"
	"path/filepath"
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
	fill(img, image.Rect(40, 118, Width-40, 120), divider)
	r.limits(img, stats.Limits, now)
	return img
}

// tokens draws Today, Month and All in one row: label, tokens and cost side by side.
func (r *Renderer) tokens(img *image.RGBA, periods usage.Periods) {
	label, value, cost := r.face(false, 28), r.face(true, 48), r.face(false, 36)
	for i, p := range []struct {
		label  string
		period usage.Period
	}{{"Today", periods.Today}, {"Month", periods.Month}, {"All", periods.AllTime}} {
		x := 40 + i*620
		r.text(img, label, dim, x, 84, p.label)
		x += measure(label, p.label) + 20
		tokens := commas(fmt.Sprint(p.period.TotalTokens))
		r.text(img, value, text, x, 84, tokens)
		x += measure(value, tokens) + 20
		r.text(img, cost, accent, x, 84, usd(p.period.CostUSD))
	}
}

// group is one contract: the windows of a provider that show a meter.
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
			if w.ShowMeter {
				g.windows = append(g.windows, w)
			}
		}
		if len(g.windows) > 0 {
			out = append(out, g)
		}
	}
	return out
}

const (
	limitsTop    = 136
	columns      = 5
	columnGap    = 40
	columnWidth  = (Width - 80 - (columns-1)*columnGap) / columns
	headerHeight = 66
	rowHeight    = 64
	barRows      = 4
	stackGap     = 36
)

// height is the drawn height of a group: its header and up to four bar rows.
func (g group) height() int { return headerHeight + (min(len(g.windows), barRows)-1)*rowHeight + 42 }

// layout places groups into columns in Hub order. A group with a single window is stacked
// under the earlier single-window groups while their column has room.
func layout(gs []group) [][]group {
	var out [][]group
	singles, used := -1, 0
	for _, g := range gs {
		if len(g.windows) == 1 && singles >= 0 && used+stackGap+g.height() <= Height-limitsTop {
			out[singles] = append(out[singles], g)
			used += stackGap + g.height()
			continue
		}
		if len(out) == columns {
			break
		}
		if len(g.windows) == 1 {
			singles, used = len(out), g.height()
		}
		out = append(out, []group{g})
	}
	return out
}

// limits draws contracts left to right, each with its name and plan above its bars.
func (r *Renderer) limits(img *image.RGBA, limits usage.Limits, now time.Time) {
	for i, column := range layout(groups(limits)) {
		x, y := 40+i*(columnWidth+columnGap), limitsTop
		for _, g := range column {
			r.text(img, r.face(true, 28), text, x, y+26, truncate(r.face(true, 28), g.name, columnWidth))
			r.text(img, r.face(false, 20), dim, x, y+52, truncate(r.face(false, 20), g.plan, columnWidth))
			r.bars(img, g, x, y+headerHeight, now)
			y += g.height() + stackGap
		}
	}
}

// bars draws up to four windows as labelled bars. The times until reset share one right edge
// so they line up across rows.
func (r *Renderer) bars(img *image.RGBA, g group, x, y int, now time.Time) {
	big, small := r.face(true, 24), r.face(false, 18)
	resetRight := x + columnWidth - measure(big, "100%") - 16
	for i, w := range g.windows {
		if i == barRows {
			break
		}
		top := y + i*rowHeight
		percent := "—"
		if w.RemainingPercent != nil {
			percent = fmt.Sprintf("%.0f%%", *w.RemainingPercent)
		}
		r.text(img, big, text, x+columnWidth-measure(big, percent), top+22, percent)
		reset := ""
		if w.ResetsAt != nil {
			reset = remaining(w.ResetsAt.Sub(now))
		}
		rw := measure(small, reset)
		r.text(img, small, dim, resetRight-rw, top+22, reset)
		r.text(img, small, text, x, top+22, truncate(small, w.Label, resetRight-rw-12-x))
		fill(img, image.Rect(x, top+32, x+columnWidth, top+42), track)
		if w.RemainingPercent != nil {
			v := min(max(*w.RemainingPercent, 0), 100)
			fill(img, image.Rect(x, top+32, x+int(float64(columnWidth)*v/100), top+42), meterColor(v))
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
