package display

import (
	"math"
	"slices"
	"strings"
	"time"

	"token-monitor-turzx/internal/usage"
)

// Options describe the image destination and the compact display controls.
type Options struct {
	// source comes from the latest state, not from a display setting.
	source             string
	DeviceID           string
	DeviceConnected    bool
	Compact            bool
	Orientation        string
	Interval           time.Duration
	SkipFull5hServices bool
	Style              Style
	ServiceContent     *serviceSelection
}

type compactPage struct {
	key      string
	provider usage.Provider
	panel    panel
	windows  []usage.Window
	content  ServiceContent
	message  string
}

func compactPages(stats *usage.Stats, options Options) ([]compactPage, string) {
	if stats == nil {
		return nil, ""
	}
	order := serviceNames(stats, options.source)
	byService := map[string][]usage.Provider{}
	for _, p := range stats.Limits.Providers {
		p.Windows = slices.DeleteFunc(slices.Clone(p.Windows), func(w usage.Window) bool { return !w.ShowMeter })
		name := serviceID(options.source, p.Provider)
		byService[name] = append(byService[name], p)
	}
	var pages []compactPage
	selected := false
	for _, name := range order {
		content := serviceContent(options.ServiceContent, options.source, name)
		if !*content.Enabled {
			continue
		}
		selected = true
		contracts := byService[name]
		full, has5h := true, false
		for _, p := range contracts {
			for _, w := range p.Windows {
				if minutes, known := lengthOf(w); known && minutes == 300 {
					has5h = true
					if w.RemainingPercent == nil || *w.RemainingPercent != 100 {
						full = false
					}
				}
			}
		}
		if options.SkipFull5hServices && has5h && full {
			continue
		}
		if !content.ShowLimits {
			pages = append(pages, compactPage{key: name + "\x1etokens", provider: usage.Provider{Provider: name}, content: content})
			continue
		}
		before := len(pages)
		slices.SortStableFunc(contracts, func(a, b usage.Provider) int {
			lowest := func(p usage.Provider) float64 {
				v := math.Inf(1)
				for _, w := range p.Windows {
					if w.RemainingPercent != nil {
						v = min(v, *w.RemainingPercent)
					}
				}
				return v
			}
			aLow, bLow := lowest(a), lowest(b)
			if aLow < bLow {
				return -1
			}
			if aLow > bLow {
				return 1
			}
			return 0
		})
		for _, p := range contracts {
			pageProvider := p
			pageProvider.Provider = name
			if options.Style == Bars {
				for start := 0; start < len(p.Windows); start += 3 {
					ws := p.Windows[start:min(start+3, len(p.Windows))]
					pages = append(pages, compactPage{key: compactKey(p, ws), provider: pageProvider, windows: ws, content: content})
				}
			} else {
				ps := panels(usage.Limits{Providers: []usage.Provider{p}})
				if len(ps) == 0 {
					continue
				}
				pn := ps[0]
				for start := 0; start < len(pn.circles); start += 3 {
					part := pn
					part.circles = pn.circles[start:min(start+3, len(pn.circles))]
					var ws []usage.Window
					for _, c := range part.circles {
						ws = append(ws, c.windows...)
					}
					pages = append(pages, compactPage{key: compactKey(p, ws), provider: pageProvider, panel: part, content: content})
				}
			}
		}
		if len(pages) == before {
			pages = append(pages, compactPage{key: name + "\x1eempty", provider: usage.Provider{Provider: name}, content: content, message: "No usage limits to display"})
		}
	}
	if len(pages) == 0 {
		if len(order) == 0 {
			return nil, "No services to display"
		}
		if !selected {
			return nil, "No services selected"
		}
		return nil, "All services skipped"
	}
	return pages, ""
}

func compactKey(p usage.Provider, ws []usage.Window) string {
	keys := []string{p.Provider, p.AccountLabel, p.PlanLabel}
	for _, w := range ws {
		keys = append(keys, windowKey(p, w))
	}
	return strings.Join(keys, "\x1e")
}

// rotation belongs to the drawing loop. Data updates never restart its deadline.
type rotation struct {
	options  Options
	pages    []compactPage
	index    int
	deadline time.Time
}

func (r *rotation) update(pages []compactPage, options Options, now time.Time) {
	before, next := r.options, options
	before.DeviceConnected, next.DeviceConnected = false, false
	reset := next != before || len(r.pages) == 0
	index := 0
	if !reset && len(pages) > 0 {
		// Keep the current page; if removed, choose the first surviving successor.
		for offset := 0; offset < len(r.pages); offset++ {
			key := r.pages[(r.index+offset)%len(r.pages)].key
			found := slices.IndexFunc(pages, func(p compactPage) bool { return p.key == key })
			if found >= 0 {
				index = found
				break
			}
		}
	}
	r.options, r.pages, r.index = options, pages, index
	if len(pages) == 0 {
		r.deadline = time.Time{}
		return
	}
	if reset {
		r.deadline = now.Add(options.Interval)
	}
	if !now.Before(r.deadline) {
		r.index = (r.index + 1) % len(pages)
		r.deadline = now.Add(options.Interval)
	}
}
