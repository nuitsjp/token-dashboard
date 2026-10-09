package display

import (
	"fmt"
	"image"
	"image/color"
	"image/draw"
	"math"
	"strings"
	"time"

	xdraw "golang.org/x/image/draw"
	"golang.org/x/image/font"
	"token-monitor-turzx/internal/usage"
)

func (r *Renderer) renderCompact(stats *usage.Stats, source string, options Options, page *compactPage, number, total int, message string, now time.Time) *image.RGBA {
	w, h := 480, 320
	portrait := options.Orientation == "Portrait" || options.Orientation == "ReversePortrait"
	if portrait {
		w, h = 320, 480
	}
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	draw.Draw(img, img.Bounds(), image.NewUniform(background), image.Point{}, draw.Src)
	if stats == nil {
		message = "Waiting for Hub"
		if source == "Local" {
			message = "Waiting for local usage"
		}
	}
	if page == nil {
		r.compactMessage(img, img.Bounds(), message)
		return img
	}
	full := image.Rect(8, 8, w-8, h-8)
	tokens, limits := full, full
	if page.content.ShowTokens && page.content.ShowLimits {
		tokens, limits = image.Rect(8, 8, 210, h-8), image.Rect(218, 8, w-8, h-8)
		if portrait {
			tokens, limits = image.Rect(8, 8, w-8, 184), image.Rect(8, 192, w-8, h-8)
		}
	}
	if page.content.ShowTokens {
		r.compactTokens(img, stats.Periods, page.provider.Provider, source, tokens, portrait, !page.content.ShowLimits)
	}
	footerRect := tokens
	if page.content.ShowLimits {
		r.compactPanel(img, limits)
		r.compactHeader(img, page.provider, limits)
		cell := image.Rect(limits.Min.X+12, limits.Min.Y+72, limits.Max.X-12, limits.Max.Y-30)
		if page.message != "" {
			r.compactMessage(img, cell, page.message)
		} else if options.Style == Bars {
			r.compactBars(img, page.windows, cell, now)
		} else {
			n := len(page.panel.circles)
			grid := page.content.ShowTokens && n == 3
			vertical := portrait && !page.content.ShowTokens || !portrait && page.content.ShowTokens
			if grid {
				vertical = false
			}
			for i, c := range page.panel.circles {
				part := image.Rect(cell.Min.X+i*cell.Dx()/n, cell.Min.Y, cell.Min.X+(i+1)*cell.Dx()/n, cell.Max.Y)
				if grid {
					x, y := cell.Min.X+i*cell.Dx()/2, cell.Min.Y
					if i == 2 {
						x, y = cell.Min.X+cell.Dx()/4, cell.Min.Y+cell.Dy()/2
					}
					part = image.Rect(x, y, x+cell.Dx()/2, y+cell.Dy()/2)
				} else if vertical {
					part = image.Rect(cell.Min.X, cell.Min.Y+i*cell.Dy()/n, cell.Max.X, cell.Min.Y+(i+1)*cell.Dy()/n)
				}
				r.compactCircle(img, c, page.panel.groups > 1, part, vertical, grid, now)
			}
		}
		footerRect = limits
	}
	footer := fmt.Sprintf("%d / %d", number, total)
	face := r.face(false, 14)
	r.text(img, face, dim, footerRect.Max.X-12-measure(face, footer), footerRect.Max.Y-9, footer)
	return img
}

func (r *Renderer) compactPanel(img *image.RGBA, rect image.Rectangle) {
	roundRect(img, rect, 12, divider)
	roundRect(img, rect.Inset(1), 11, panelFill)
}

func (r *Renderer) compactMessage(img *image.RGBA, rect image.Rectangle, message string) {
	size := 18.0
	face := r.face(false, size)
	for size > 1 && measure(face, message) > rect.Dx()-12 {
		size--
		face = r.face(false, size)
	}
	r.text(img, face, dim, rect.Min.X+(rect.Dx()-measure(face, message))/2, rect.Min.Y+rect.Dy()/2, message)
}

func (r *Renderer) compactHeader(img *image.RGBA, provider usage.Provider, rect image.Rectangle) {
	x, y := rect.Min.X+12, rect.Min.Y
	if icon, ok := r.icons[strings.ToLower(provider.Provider)]; ok {
		xdraw.CatmullRom.Scale(img, image.Rect(x, y+11, x+26, y+37), icon, icon.Bounds(), xdraw.Over, nil)
	} else {
		roundRect(img, image.Rect(x, y+11, x+26, y+37), 5, track)
		initial := "?"
		if name := []rune(strings.TrimSpace(provider.Provider)); len(name) > 0 {
			initial = strings.ToUpper(string(name[0]))
		}
		face := r.face(true, 18)
		r.text(img, face, text, x+(26-measure(face, initial))/2, y+31, initial)
	}
	x += 36
	name := r.face(true, 22)
	r.text(img, name, text, x, y+34, truncate(name, provider.Provider, rect.Max.X-x-12))
	plan := provider.PlanLabel
	if plan == "" {
		plan = provider.AccountLabel
	}
	if plan != "" {
		r.text(img, r.face(false, 16), dim, rect.Min.X+12, y+57, truncate(r.face(false, 16), plan, rect.Dx()-24))
	}
}

// Values are fitted in full using the wide renderer's numeric formatting.
func (r *Renderer) compactValue(img *image.RGBA, value string, bold bool, size float64, c color.Color, x, baseline, width int) {
	face := r.face(bold, size)
	for size > 1 && measure(face, value) > width {
		size--
		face = r.face(bold, size)
	}
	r.text(img, face, c, x+width-measure(face, value), baseline, value)
}

func (r *Renderer) compactTokens(img *image.RGBA, periods usage.Periods, service, source string, rect image.Rectangle, portrait, only bool) {
	r.compactPanel(img, rect)
	x, width := rect.Min.X+12, rect.Dx()-24
	captionY := rect.Min.Y + 18
	if only {
		r.compactHeader(img, usage.Provider{Provider: service}, rect)
		captionY = rect.Min.Y + 54
	}
	r.text(img, r.face(false, 11), dim, x, captionY, "Service total")
	client := tokenServiceID(source, service)
	blocks := []struct {
		label  string
		period usage.Period
	}{{"Today", periods.Today}, {"Month", periods.Month}, {"All", periods.AllTime}}
	for i, b := range blocks {
		tokens, cost := "—", "—"
		if b.period.ClientBreakdown != nil {
			if value, ok := b.period.Clients[client]; ok {
				tokens = commas(fmt.Sprint(value))
			}
			if value, ok := b.period.ClientCosts[client]; ok {
				cost = usd(value)
			}
		}
		if only {
			step := (rect.Dy() - 88) / 3
			top := rect.Min.Y + 66 + i*step
			r.text(img, r.face(false, 16), dim, x, top+18, b.label)
			r.compactValue(img, cost, false, 18, accent, x+64, top+18, width-64)
			r.compactValue(img, tokens, true, 30, text, x, top+min(step-8, 52), width)
		} else if portrait {
			top := rect.Min.Y + 24 + i*48
			r.text(img, r.face(false, 13), dim, x, top+18, b.label)
			r.compactValue(img, cost, false, 15, accent, x+56, top+18, width-56)
			r.compactValue(img, tokens, true, 20, text, x, top+42, width)
		} else {
			top := rect.Min.Y + 22 + i*91
			r.text(img, r.face(false, 16), dim, x, top+22, b.label)
			r.compactValue(img, tokens, true, 24, text, x, top+50, width)
			r.compactValue(img, cost, false, 18, accent, x, top+76, width)
		}
	}
}

func (r *Renderer) compactCircle(img *image.RGBA, c circle, showGroup bool, cell image.Rectangle, vertical, grid bool, now time.Time) {
	cx := float64(cell.Min.X + cell.Dx()/2)
	radius := min(83.0, float64(cell.Dx()-8)/2, float64(cell.Dy()-64)/2)
	if grid {
		detailHeight := len(c.windows) * 12
		if showGroup {
			detailHeight += 12
		}
		// The 270-degree arc opens below its centre: its lower edge is at
		// 0.707 radii, plus half the stroke. Reserve measured text rows below it.
		radius = min(83.0, float64(cell.Dx()-8)/2, float64(cell.Dy()-detailHeight-8)/1.762)
	}
	cy := float64(cell.Min.Y) + radius + 4
	details := cell
	if vertical {
		radius = min(83.0, float64(cell.Dy()-8)/2, float64(cell.Dx())*0.23)
		cx, cy = float64(cell.Min.X)+radius+4, float64(cell.Min.Y+cell.Dy()/2)
		details.Min.X, details.Max.X = int(cx+radius)+10, cell.Max.X-4
	}
	k := radius / 83
	stroke, ringGap := 9*k, 17*k
	for i, w := range c.windows {
		rr := radius - float64(i)*ringGap
		fillPoly(img, track, arcPoly(cx, cy, rr, stroke, 135, 270))
		if w.RemainingPercent != nil {
			fillPoly(img, gaugeColor(w, now), arcPoly(cx, cy, rr, stroke, 135, 270*min(100, max(0, *w.RemainingPercent))/100))
		}
	}
	bigSize, smallSize := max(11.0, math.Round(22*k)), max(7.0, math.Round(14*k))
	pairWidth := int(2 * (radius - float64(len(c.windows)-1)*ringGap - stroke - 2))
	gap := max(3, int(6*k))
	pitch := max(13, int(27*k))
	centerBaseline := int(bigSize / 3)
	if grid {
		bigSize, smallSize = 14, 9
		inner := radius - float64(len(c.windows)-1)*ringGap - stroke - 2
		for {
			big, small := r.face(true, bigSize), r.face(false, smallSize)
			top, bottom, widest := 0, 0, 0
			for _, w := range c.windows {
				percent := "—"
				if w.RemainingPercent != nil {
					percent = fmt.Sprintf("%.0f%%", math.Round(*w.RemainingPercent))
				}
				label := windowLabel(w)
				for _, bounds := range []struct {
					face  font.Face
					value string
				}{{big, percent}, {small, label}} {
					box, _ := font.BoundString(bounds.face, bounds.value)
					top, bottom = min(top, box.Min.Y.Floor()), max(bottom, box.Max.Y.Ceil())
				}
				widest = max(widest, measure(big, percent)+gap+measure(small, label))
			}
			height := bottom - top
			pitch = height + 2
			block := height*len(c.windows) + 2*(len(c.windows)-1)
			centerBaseline = -block/2 - top
			extent := float64((block + 1) / 2)
			if extent < inner {
				pairWidth = int(2 * math.Sqrt(inner*inner-extent*extent))
				if widest <= pairWidth {
					break
				}
			}
			if bigSize <= 3 {
				break
			}
			bigSize -= 0.5
			smallSize = max(3, bigSize*9/14)
		}
	}
	for i, w := range c.windows {
		percent := "—"
		if w.RemainingPercent != nil {
			percent = fmt.Sprintf("%.0f%%", math.Round(*w.RemainingPercent))
		}
		label := windowLabel(w)
		big, small := r.face(true, bigSize), r.face(false, smallSize)
		for size := bigSize; size > 3 && measure(big, percent)+gap+measure(small, label) > pairWidth; {
			size -= 0.5
			big, small = r.face(true, size), r.face(false, max(3, size*smallSize/bigSize))
		}
		x := int(cx) - (measure(big, percent)+gap+measure(small, label))/2
		y := int(cy) + int(bigSize/3)
		if grid {
			y = int(cy) + centerBaseline + i*pitch
		} else if len(c.windows) == 2 {
			y -= pitch / 2
			y += i * pitch
		}
		r.text(img, big, text, x, y, percent)
		r.text(img, small, dim, x+measure(big, percent)+gap, y, label)
	}
	if showGroup {
		face := r.face(false, max(8, math.Round(14*k)))
		label := truncate(face, groupOf(c.windows[0]), cell.Dx()-4)
		if grid {
			face = r.face(false, 10)
			label = truncate(face, groupOf(c.windows[0]), cell.Dx()-4)
			r.text(img, face, text, int(cx)-measure(face, label)/2, cell.Max.Y-4-len(c.windows)*12, label)
		} else if vertical {
			face = r.face(false, 14)
			label = truncate(face, groupOf(c.windows[0]), details.Dx())
			height := 18 + len(c.windows)*20
			r.text(img, face, text, details.Min.X, cell.Min.Y+(cell.Dy()-height)/2+14, label)
		} else {
			r.text(img, face, text, int(cx)-measure(face, label)/2, int(cy+radius*0.78), label)
		}
	}
	for i, w := range c.windows {
		reset, period := resetIn(w, now), windowLabel(w)
		size := max(9.0, math.Round(16*k))
		if vertical {
			size = 13
		} else if grid {
			size = 10
		}
		label, when := r.face(true, size), r.face(false, size)
		icon, gap := max(3, int(7*k)), max(3, int(8*k))
		for size > 3 && measure(label, period)+gap*2+icon*2+measure(when, reset) > details.Dx()-4 {
			size -= 0.5
			label, when = r.face(true, size), r.face(false, size)
		}
		labelWidth := measure(label, period)
		x := int(cx) - (labelWidth+gap*2+icon*2+measure(when, reset))/2
		y := cell.Max.Y - 11 - (len(c.windows)-1-i)*max(14, int(22*k))
		if grid {
			y = cell.Max.Y - 4 - (len(c.windows)-1-i)*12
		} else if vertical {
			height, offset := len(c.windows)*20, 0
			if showGroup {
				height += 18
				offset = 18
			}
			x, y = details.Min.X, cell.Min.Y+(cell.Dy()-height)/2+offset+14+i*20
		}
		r.text(img, label, dim, x, y, period)
		clock(img, float64(x+labelWidth+gap+icon), float64(y)-size*0.4, float64(icon))
		r.text(img, when, text, x+labelWidth+gap*2+icon*2, y, reset)
	}
}

func (r *Renderer) compactBars(img *image.RGBA, ws []usage.Window, cell image.Rectangle, now time.Time) {
	row := cell.Dy() / 3
	for i, w := range ws {
		x, top := cell.Min.X, cell.Min.Y+i*row
		label := w.Label
		if label == "" {
			label = w.Kind
		}
		compact := row < 64
		nameSize, timeSize, percentSize := 16.0, 13.0, 20.0
		baseline, barTop := 42, 52
		if compact {
			nameSize, timeSize, percentSize = 14, 12, 18
			baseline, barTop = 36, 45
		}
		face := r.face(false, nameSize)
		r.text(img, face, text, x, top+18, truncate(face, label, cell.Dx()))
		percent, reset := "—", "—"
		if w.RemainingPercent != nil {
			percent = fmt.Sprintf("%.0f%%", *w.RemainingPercent)
		}
		if w.ResetsAt != nil {
			reset = remaining(w.ResetsAt.Sub(now))
		}
		r.compactValue(img, reset, false, timeSize, dim, x, top+baseline, cell.Dx()-measure(r.face(true, percentSize), percent)-12)
		r.text(img, r.face(true, percentSize), text, cell.Max.X-measure(r.face(true, percentSize), percent), top+baseline, percent)
		bar := image.Rect(x, top+barTop, cell.Max.X, top+barTop+8)
		fill(img, bar, track)
		if w.RemainingPercent != nil {
			bar.Max.X = x + int(float64(cell.Dx())*min(100, max(0, *w.RemainingPercent))/100)
			fill(img, bar, gaugeColor(w, now))
		}
	}
}
