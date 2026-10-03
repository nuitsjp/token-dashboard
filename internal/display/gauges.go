package display

import (
	"cmp"
	"embed"
	"fmt"
	"image"
	"image/color"
	_ "image/png"
	"math"
	"regexp"
	"slices"
	"strings"
	"time"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/vector"

	"token-monitor-turzx/internal/usage"
)

//go:embed icons/*.png
var iconFiles embed.FS

var (
	panelFill   = color.RGBA{0x15, 0x18, 0x21, 0xff}
	gaugeNormal = color.RGBA{0x90, 0x85, 0xe9, 0xff}
	gaugeWarn   = color.RGBA{0xfa, 0xb2, 0x19, 0xff}
	gaugeBad    = color.RGBA{0xf0, 0x61, 0x6d, 0xff}
)

const (
	cellWidth  = 240
	cellGap    = 12
	panelTop   = 30
	panelGap   = 14
	gaugesLeft = 40
	panelPad   = 10
	// Today, Month and All run across the top; the panels keep the same margin above and below.
	tokenStrip = 56
)

// loadIcons decodes the provider icons shipped with the app, keyed by the lower-case provider name.
func loadIcons() map[string]image.Image {
	icons := map[string]image.Image{}
	entries, _ := iconFiles.ReadDir("icons")
	for _, e := range entries {
		f, err := iconFiles.Open("icons/" + e.Name())
		if err != nil {
			continue
		}
		img, _, err := image.Decode(f)
		f.Close()
		if err == nil {
			icons[strings.TrimSuffix(e.Name(), ".png")] = img
		}
	}
	return icons
}

var kindMinutes = map[string]float64{"session": 300, "daily": 1440, "weekly": 10080, "billing": 43200}

var windowWord = regexp.MustCompile(`(?i)\s*\b(5-hour|5h|session|weekly|daily|monthly)$`)

func groupOf(w usage.Window) string {
	return strings.TrimSpace(windowWord.ReplaceAllString(w.Label, ""))
}

// lengthOf is the window length in minutes: what the source reports, else the fixed table by kind.
func lengthOf(w usage.Window) (float64, bool) {
	if w.WindowMinutes != nil {
		return *w.WindowMinutes, true
	}
	m, ok := kindMinutes[w.Kind]
	return m, ok
}

func windowLabel(w usage.Window) string {
	m, ok := lengthOf(w)
	switch {
	case !ok:
		return w.Kind
	case m >= 28*1440 && m <= 31*1440:
		return "1mo"
	case m < 1440 && math.Mod(m, 60) == 0:
		return fmt.Sprintf("%dh", int(m/60))
	case m < 1440:
		return fmt.Sprintf("%dm", int(m))
	default:
		return fmt.Sprintf("%dd", int(math.Round(m/1440)))
	}
}

// resetIn formats the time until a reset: "2h 13m", "13m" below an hour, "3d 4h" from a day, "—" when unknown.
func resetIn(w usage.Window, now time.Time) string {
	if w.ResetsAt == nil {
		return "—"
	}
	minutes := max(0, int(w.ResetsAt.Sub(now)/time.Minute))
	switch {
	case minutes >= 1440:
		return fmt.Sprintf("%dd %dh", minutes/1440, minutes%1440/60)
	case minutes >= 60:
		return fmt.Sprintf("%dh %dm", minutes/60, minutes%60)
	default:
		return fmt.Sprintf("%dm", minutes)
	}
}

// gaugeColor is the worse of the pace state and the remaining state.
func gaugeColor(w usage.Window, now time.Time) color.Color {
	if w.RemainingPercent == nil {
		return gaugeNormal
	}
	remaining := *w.RemainingPercent
	level := 0
	switch {
	case remaining < 25:
		level = 2
	case remaining <= 40:
		level = 1
	}
	if length, ok := lengthOf(w); ok && w.ResetsAt != nil {
		ideal := min(1, max(0, w.ResetsAt.Sub(now).Minutes()/length))
		if ideal > 0 {
			switch pace := remaining / (ideal * 100); {
			case pace < 0.5:
				level = max(level, 2)
			case pace < 0.8:
				level = max(level, 1)
			}
		}
	}
	return [...]color.Color{gaugeNormal, gaugeWarn, gaugeBad}[level]
}

type circle struct{ windows []usage.Window }

type panel struct {
	provider, name, plan string
	circles              []circle
	groups               int
	lowest               float64
}

// panels builds one panel per contract: a circle per window group, two windows to a circle, the
// shorter window outside. Contracts with the smallest remaining percent come first.
func panels(limits usage.Limits) []panel {
	var out []panel
	for _, p := range limits.Providers {
		pn := panel{provider: p.Provider, name: p.Provider, plan: p.PlanLabel, lowest: math.Inf(1)}
		if pn.plan == "" {
			pn.plan = p.AccountLabel
		}
		var order []string
		byGroup := map[string][]usage.Window{}
		for _, w := range p.Windows {
			if !w.ShowMeter {
				continue
			}
			g := groupOf(w)
			if _, seen := byGroup[g]; !seen {
				order = append(order, g)
			}
			byGroup[g] = append(byGroup[g], w)
			if w.RemainingPercent != nil {
				pn.lowest = min(pn.lowest, *w.RemainingPercent)
			}
		}
		for _, g := range order {
			ws := slices.Clone(byGroup[g])
			slices.SortStableFunc(ws, func(a, b usage.Window) int {
				la, oka := lengthOf(a)
				lb, okb := lengthOf(b)
				if !oka {
					la = math.Inf(1)
				}
				if !okb {
					lb = math.Inf(1)
				}
				return cmp.Compare(la, lb)
			})
			for i := 0; i < len(ws); i += 2 {
				pn.circles = append(pn.circles, circle{windows: ws[i:min(i+2, len(ws))]})
			}
		}
		pn.groups = len(order)
		if len(pn.circles) > 0 {
			out = append(out, pn)
		}
	}
	slices.SortStableFunc(out, func(a, b panel) int { return cmp.Compare(a.lowest, b.lowest) })
	return out
}

func (p panel) width() int { return len(p.circles)*cellWidth + (len(p.circles)-1)*cellGap + 2*panelPad }

// gauges keeps the default margins unless reducing them fits more whole contracts.
// A panel that does not fit even without margins, and every one after it, is not shown.
func (r *Renderer) gauges(img *image.RGBA, stats *usage.Stats, now time.Time) {
	r.tokenStrip(img, stats.Periods)
	top := panelTop + tokenStrip
	height := Height - top - panelTop
	ps := panels(stats.Limits)
	width, count := 0, 0
	for _, p := range ps {
		next := width + p.width()
		if count > 0 {
			next += panelGap
		}
		if next > Width {
			break
		}
		width = next
		count++
	}
	x := min(gaugesLeft, (Width-width)/2)
	for _, p := range ps[:count] {
		r.panel(img, p, x, top, height, now)
		x += p.width() + panelGap
	}
}

func (r *Renderer) panel(img *image.RGBA, p panel, x, top, height int, now time.Time) {
	w := p.width()
	rect := image.Rect(x, top, x+w, top+height)
	roundRect(img, rect, 16, divider)
	roundRect(img, rect.Inset(1), 15, panelFill)
	// Heading: icon, provider and plan.
	name, plan := r.face(true, 26), r.face(false, 22)
	hx, base := x+panelPad+4, top+38
	if icon, ok := r.icons[strings.ToLower(p.provider)]; ok {
		dst := image.Rect(hx, base-26, hx+28, base+2)
		xdraw.CatmullRom.Scale(img, dst, icon, icon.Bounds(), xdraw.Over, nil)
		hx += 38
	}
	n := truncate(name, p.name, x+w-panelPad-hx)
	r.text(img, name, text, hx, base, n)
	if px := hx + measure(name, n) + 10; px < x+w-panelPad {
		r.text(img, plan, dim, px, base, truncate(plan, p.plan, x+w-panelPad-px))
	}
	for i, c := range p.circles {
		cellX := x + panelPad + i*(cellWidth+cellGap)
		r.circle(img, c, p.groups > 1, cellX, top, height, now)
	}
}

// circle draws one circle in a cell: the arcs, the percentages in the middle, the group name in the
// gap below and, under it, a row per window with its label and time until reset.
func (r *Renderer) circle(img *image.RGBA, c circle, showGroup bool, cellX, panelY, height int, now time.Time) {
	// k fits the cell width, or the panel height when that is smaller. u scales what was drawn for
	// the first, smaller cell. The block of circle and rows is centred vertically below the heading.
	const blockPerK = 182 + (44+32+8)/0.92
	avail := float64(height - headerHeight - panelPad)
	k := min(float64(cellWidth-16)/200, avail/blockPerK)
	u := k / 0.92
	block := blockPerK * k
	top := panelY + headerHeight + (height-headerHeight-panelPad-int(block))/2
	ox, oy := float64(cellX)+(cellWidth-200*k)/2, float64(top)-2*k
	cx, cy := ox+100*k, oy+100*k
	for i, w := range c.windows {
		radius := 90 * k
		if i == 1 {
			radius = 72 * k
		}
		fillPoly(img, track, arcPoly(cx, cy, radius, 11*k, 135, 270))
		if w.RemainingPercent != nil {
			v := min(max(*w.RemainingPercent, 0), 100)
			fillPoly(img, gaugeColor(w, now), arcPoly(cx, cy, radius, 11*k, 135, 270*v/100))
		}
	}
	big, small := r.face(true, math.Round(27*k)), r.face(false, math.Round(18*k))
	ys := []float64{108}
	if len(c.windows) == 2 {
		ys = []float64{92, 123}
	}
	for i, w := range c.windows {
		percent := "—"
		if w.RemainingPercent != nil {
			percent = fmt.Sprintf("%.0f%%", math.Round(*w.RemainingPercent))
		}
		label := windowLabel(w)
		total := measure(big, percent) + 6 + measure(small, label)
		tx, ty := int(cx)-total/2, int(oy+ys[i]*k)
		r.text(img, big, text, tx, ty, percent)
		r.text(img, small, dim, tx+measure(big, percent)+6, ty, label)
	}
	if showGroup && len(c.windows) > 0 {
		face := r.face(false, math.Round(18*k))
		g := truncate(face, groupOf(c.windows[0]), cellWidth-16)
		y := 190.0
		if len(c.windows) == 2 {
			y = 186
		}
		r.text(img, face, text, int(cx)-measure(face, g)/2, int(oy+y*k), g)
	}
	label, when := r.face(true, math.Round(20*u)), r.face(false, math.Round(20*u))
	labelWidth := 0
	for _, w := range c.windows {
		labelWidth = max(labelWidth, measure(label, windowLabel(w)))
	}
	for i, w := range c.windows {
		t := resetIn(w, now)
		gap, icon := int(12*u), int(16*u)
		total := labelWidth + gap + icon + int(8*u) + measure(when, t)
		x := int(cx) - total/2
		y := int(oy+182*k) + int(44*u) + i*int(32*u)
		r.text(img, label, dim, x, y, windowLabel(w))
		clock(img, float64(x+labelWidth+gap+int(8*u)), float64(y)-7*u, 8*u)
		r.text(img, when, text, x+labelWidth+gap+icon+int(8*u), y, t)
	}
}

// tokenStrip draws Today, Month and All side by side from the left margin: the period, the tokens
// and the cost, with a fixed gap between the three.
func (r *Renderer) tokenStrip(img *image.RGBA, periods usage.Periods) {
	blocks := []struct {
		label  string
		period usage.Period
	}{{"Today", periods.Today}, {"Month", periods.Month}, {"All", periods.AllTime}}
	label, cost, tokens := r.face(false, 20), r.face(false, 22), r.face(true, 34)
	x, base := gaugesLeft, panelTop+34
	for _, b := range blocks {
		r.text(img, label, dim, x, base, b.label)
		x += measure(label, b.label) + 14
		t := commas(fmt.Sprint(b.period.TotalTokens))
		r.text(img, tokens, text, x, base, t)
		x += measure(tokens, t) + 14
		c := usd(b.period.CostUSD)
		r.text(img, cost, accent, x, base, c)
		x += measure(cost, c) + 64
	}
}

// clock draws a small clock face centred on (cx, cy).
func clock(img *image.RGBA, cx, cy, radius float64) {
	fillPoly(img, text, arcPoly(cx, cy, radius-1, 2, 0, 360))
	fillPoly(img, text, linePoly(cx, cy-radius*0.55, cx, cy, 2))
	fillPoly(img, text, linePoly(cx, cy, cx+radius*0.45, cy+radius*0.3, 2))
}

type fpt struct{ x, y float64 }

// polar is the point at angle deg and distance r from (cx, cy). Angles run clockwise on the screen,
// and 0 points to the right.
func polar(cx, cy, r, deg float64) fpt {
	a := deg * math.Pi / 180
	return fpt{cx + r*math.Cos(a), cy + r*math.Sin(a)}
}

// arcPoly outlines a stroke of width w along a circle of radius r, from angle start through sweep
// degrees clockwise, with round ends.
func arcPoly(cx, cy, r, w, start, sweep float64) []fpt {
	steps := max(2, int(sweep/3))
	var pts []fpt
	for i := 0; i <= steps; i++ {
		pts = append(pts, polar(cx, cy, r+w/2, start+sweep*float64(i)/float64(steps)))
	}
	end := start + sweep
	ce, cb := polar(cx, cy, r, end), polar(cx, cy, r, start)
	for i := 1; i < 12; i++ {
		pts = append(pts, polar(ce.x, ce.y, w/2, end+180*float64(i)/12))
	}
	for i := steps; i >= 0; i-- {
		pts = append(pts, polar(cx, cy, r-w/2, start+sweep*float64(i)/float64(steps)))
	}
	for i := 1; i < 12; i++ {
		pts = append(pts, polar(cb.x, cb.y, w/2, start+180+180*float64(i)/12))
	}
	return pts
}

func linePoly(x1, y1, x2, y2, w float64) []fpt {
	dx, dy := x2-x1, y2-y1
	n := math.Hypot(dx, dy)
	if n == 0 {
		return nil
	}
	nx, ny := -dy/n*w/2, dx/n*w/2
	return []fpt{{x1 + nx, y1 + ny}, {x2 + nx, y2 + ny}, {x2 - nx, y2 - ny}, {x1 - nx, y1 - ny}}
}

// fillPoly fills the polygon with c, anti-aliased.
func fillPoly(img *image.RGBA, c color.Color, pts []fpt) {
	if len(pts) < 3 {
		return
	}
	minX, minY, maxX, maxY := math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)
	for _, p := range pts {
		minX, minY, maxX, maxY = min(minX, p.x), min(minY, p.y), max(maxX, p.x), max(maxY, p.y)
	}
	box := image.Rect(int(math.Floor(minX))-1, int(math.Floor(minY))-1, int(math.Ceil(maxX))+1, int(math.Ceil(maxY))+1).Intersect(img.Bounds())
	if box.Empty() {
		return
	}
	z := vector.NewRasterizer(box.Dx(), box.Dy())
	for i, p := range pts {
		x, y := float32(p.x-float64(box.Min.X)), float32(p.y-float64(box.Min.Y))
		if i == 0 {
			z.MoveTo(x, y)
		} else {
			z.LineTo(x, y)
		}
	}
	z.ClosePath()
	z.Draw(img, box, image.NewUniform(c), image.Point{})
}

func roundRect(img *image.RGBA, rect image.Rectangle, radius float64, c color.Color) {
	x0, y0, x1, y1 := float64(rect.Min.X), float64(rect.Min.Y), float64(rect.Max.X), float64(rect.Max.Y)
	var pts []fpt
	for _, corner := range []struct{ cx, cy, from float64 }{{x1 - radius, y0 + radius, 270}, {x1 - radius, y1 - radius, 0}, {x0 + radius, y1 - radius, 90}, {x0 + radius, y0 + radius, 180}} {
		for i := 0; i <= 8; i++ {
			pts = append(pts, polar(corner.cx, corner.cy, radius, corner.from+90*float64(i)/8))
		}
	}
	fillPoly(img, c, pts)
}
