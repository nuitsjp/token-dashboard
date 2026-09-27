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
	fill(img, image.Rect(960, 40, 962, Height-40), divider)
	r.limits(img, stats.Limits, now)
	return img
}

func (r *Renderer) tokens(img *image.RGBA, periods usage.Periods) {
	r.text(img, r.face(true, 30), dim, 40, 64, "TOKENS")
	for i, p := range []struct {
		label  string
		period usage.Period
	}{{"Today", periods.Today}, {"Month", periods.Month}, {"All", periods.AllTime}} {
		x := 40 + i*300
		r.text(img, r.face(false, 34), dim, x, 150, p.label)
		r.text(img, r.face(true, 72), text, x, 250, compactTokens(p.period.TotalTokens))
		r.text(img, r.face(false, 50), accent, x, 345, usd(p.period.CostUSD))
	}
}

func (r *Renderer) limits(img *image.RGBA, limits usage.Limits, now time.Time) {
	r.text(img, r.face(true, 30), dim, 1000, 64, "USAGE LIMITS")
	const rows, columns, width = 4, 2, 420
	slot := 0
	for _, p := range limits.Providers {
		for _, w := range p.Windows {
			if !w.ShowMeter || slot == rows*columns {
				continue
			}
			x, y := 1000+(slot/rows)*(width+40), 88+(slot%rows)*88
			slot++
			percent := "—"
			if w.RemainingPercent != nil {
				percent = fmt.Sprintf("%.0f%%", *w.RemainingPercent)
			}
			big, small := r.face(true, 30), r.face(false, 22)
			pw := measure(big, percent)
			r.text(img, big, text, x+width-pw, y+34, percent)
			r.text(img, r.face(true, 26), text, x, y+34, truncate(r.face(true, 26), p.AccountLabel, width-pw-20))
			fill(img, image.Rect(x, y+46, x+width, y+58), track)
			if w.RemainingPercent != nil {
				v := min(max(*w.RemainingPercent, 0), 100)
				fill(img, image.Rect(x, y+46, x+int(float64(width)*v/100), y+58), meterColor(v))
			}
			reset := ""
			if w.ResetsAt != nil {
				reset = remaining(w.ResetsAt.Sub(now))
			}
			rw := measure(small, reset)
			r.text(img, small, dim, x+width-rw, y+84, reset)
			r.text(img, small, dim, x, y+84, truncate(small, w.Label, width-rw-20))
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

func compactTokens(n int64) string {
	switch {
	case n >= 1_000_000_000:
		return fmt.Sprintf("%.1fB", float64(n)/1e9)
	case n >= 1_000_000:
		return fmt.Sprintf("%.1fM", float64(n)/1e6)
	case n >= 1_000:
		return fmt.Sprintf("%.1fK", float64(n)/1e3)
	default:
		return fmt.Sprint(n)
	}
}

func usd(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	whole, cents, _ := strings.Cut(s, ".")
	var b strings.Builder
	for i, c := range whole {
		if i > 0 && (len(whole)-i)%3 == 0 {
			b.WriteByte(',')
		}
		b.WriteRune(c)
	}
	return "$" + b.String() + "." + cents
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
