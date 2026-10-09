package display

import (
	"fmt"
	"io/fs"
	"math"
	"strings"
	"time"

	"token-monitor-turzx/internal/usage"
)

// ThemeData is the prepared data shared by the HTML display themes.
type ThemeData struct {
	Available      bool            `json:"available"`
	WaitingMessage string          `json:"waitingMessage"`
	Tokens         []ThemeToken    `json:"tokens"`
	GaugePanels    []ThemeContract `json:"gaugePanels"`
	BarColumns     []ThemeColumn   `json:"barColumns"`
}

type ThemeToken struct {
	Label          string `json:"label"`
	TokensText     string `json:"tokensText"`
	CostText       string `json:"costText"`
	TokensFontSize int    `json:"tokensFontSize"`
}

type ThemeContract struct {
	Provider string        `json:"provider"`
	Plan     string        `json:"plan"`
	Icon     string        `json:"icon"`
	Span     int           `json:"span"`
	Circles  []ThemeCircle `json:"circles"`
	Windows  []ThemeWindow `json:"windows"`
}

type ThemeCircle struct {
	Name     string        `json:"name"`
	Double   bool          `json:"double"`
	ShowName bool          `json:"showName"`
	Windows  []ThemeWindow `json:"windows"`
}

type ThemeColumn struct {
	Contracts []ThemeContract `json:"contracts"`
}

type ThemeWindow struct {
	ID               string   `json:"id"`
	Label            string   `json:"label"`
	Tone             string   `json:"tone"`
	PercentText      string   `json:"percentText"`
	DurationLabel    string   `json:"durationLabel"`
	ResetText        string   `json:"resetText"`
	BarResetText     string   `json:"barResetText"`
	RemainingPercent *float64 `json:"remainingPercent"`
	HasRemaining     bool     `json:"hasRemaining"`
}

// themeData preserves the display's existing grouping, order and capacity limits.
// The caller removes hidden windows before preparing this data.
func themeData(stats *usage.Stats, now time.Time, source string) ThemeData {
	data := ThemeData{
		WaitingMessage: "Waiting for Hub", Tokens: []ThemeToken{},
		GaugePanels: []ThemeContract{}, BarColumns: []ThemeColumn{},
	}
	if source == "Local" {
		data.WaitingMessage = "Waiting for local usage"
	}
	if stats == nil {
		return data
	}
	data.Available = true
	for _, period := range []struct {
		label string
		value usage.Period
	}{{"Today", stats.Periods.Today}, {"Month", stats.Periods.Month}, {"All", stats.Periods.AllTime}} {
		data.Tokens = append(data.Tokens, ThemeToken{
			Label: period.label, TokensText: commas(fmt.Sprint(period.value.TotalTokens)),
			CostText: usd(period.value.CostUSD), TokensFontSize: 42,
		})
	}
	usedColumns := 0
	for _, p := range panels(stats.Limits) {
		if usedColumns+len(p.circles) > gaugeColumns {
			continue
		}
		contract := ThemeContract{Provider: p.provider, Plan: p.plan, Span: len(p.circles)}
		icon := strings.ToLower(p.provider) + ".png"
		if _, err := fs.Stat(iconFiles, "icons/"+icon); err == nil {
			contract.Icon = "/assets/agents/" + icon
		}
		for _, c := range p.circles {
			circle := ThemeCircle{Name: groupOf(c.windows[0]), Double: len(c.windows) == 2, ShowName: p.groups > 1}
			for _, w := range c.windows {
				window := themeWindow(w, now)
				if w.RemainingPercent != nil {
					window.PercentText = fmt.Sprintf("%.0f%%", math.Round(*w.RemainingPercent))
				}
				circle.Windows = append(circle.Windows, window)
			}
			contract.Circles = append(contract.Circles, circle)
		}
		data.GaugePanels = append(data.GaugePanels, contract)
		usedColumns += contract.Span
	}
	for _, column := range layout(groups(stats.Limits)) {
		out := ThemeColumn{}
		for _, g := range column {
			contract := ThemeContract{Provider: g.name, Plan: g.plan}
			for _, w := range g.windows {
				contract.Windows = append(contract.Windows, themeWindow(w, now))
			}
			out.Contracts = append(out.Contracts, contract)
		}
		data.BarColumns = append(data.BarColumns, out)
	}
	return data
}

func themeWindow(w usage.Window, now time.Time) ThemeWindow {
	window := ThemeWindow{
		ID: w.Kind + "/" + w.Label, Label: w.Label, Tone: "normal", PercentText: "—",
		DurationLabel: windowLabel(w), ResetText: resetIn(w, now),
		RemainingPercent: w.RemainingPercent, HasRemaining: w.RemainingPercent != nil,
	}
	if window.Label == "" {
		window.Label = w.Kind
	}
	if w.RemainingPercent != nil {
		window.PercentText = fmt.Sprintf("%.0f%%", *w.RemainingPercent)
		value := min(max(*w.RemainingPercent, 0), 100)
		window.RemainingPercent = &value
	}
	if w.ResetsAt != nil {
		window.BarResetText = remaining(w.ResetsAt.Sub(now))
	}
	switch gaugeColor(w, now) {
	case gaugeWarn:
		window.Tone = "warning"
	case gaugeBad:
		window.Tone = "danger"
	}
	return window
}
