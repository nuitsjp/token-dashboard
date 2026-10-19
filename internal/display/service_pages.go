package display

import (
	"cmp"
	"fmt"
	"io/fs"
	"math"
	"slices"
	"strings"
	"time"

	"token-monitor-turzx/internal/usage"
)

var localServiceNames = map[string]string{"claude": "Claude", "codex": "Codex", "amp": "Amp", "antigravity": "Antigravity", "copilot": "Copilot", "grok": "Grok Build", "kimi": "Kimi", "warp": "Warp/Oz"}

func serviceName(id, source string) string {
	if source == "Local" {
		if name, ok := localServiceNames[id]; ok {
			return name
		}
	}
	return id
}

type ServiceChoice struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
	Content string `json:"content"`
}

type ServicePage struct {
	Key        string        `json:"key"`
	Provider   string        `json:"provider"`
	Plan       string        `json:"plan"`
	Icon       string        `json:"icon"`
	Initial    string        `json:"initial"`
	Tokens     []ThemeToken  `json:"tokens"`
	Circles    []ThemeCircle `json:"circles"`
	Windows    []ThemeWindow `json:"windows"`
	ShowTokens bool          `json:"showTokens"`
	ShowLimits bool          `json:"showLimits"`
	Both       bool          `json:"both"`
	Message    string        `json:"message"`
	Number     int           `json:"number"`
	Count      int           `json:"count"`
}

func serviceNames(stats *usage.Stats, source string) []string {
	names := []string{}
	if stats == nil {
		return names
	}
	add := func(id string) {
		name := serviceName(id, source)
		if !slices.Contains(names, name) {
			names = append(names, name)
		}
	}
	for _, provider := range stats.Limits.Providers {
		add(provider.Provider)
	}
	tokens := []string{}
	for _, period := range []usage.Period{stats.Periods.Today, stats.Periods.Month, stats.Periods.AllTime} {
		if period.ClientBreakdown == nil {
			continue
		}
		for id := range period.Clients {
			name := serviceName(id, source)
			if !slices.Contains(names, name) && !slices.Contains(tokens, name) {
				tokens = append(tokens, name)
			}
		}
		for id := range period.ClientCosts {
			name := serviceName(id, source)
			if !slices.Contains(names, name) && !slices.Contains(tokens, name) {
				tokens = append(tokens, name)
			}
		}
	}
	slices.Sort(tokens)
	return append(names, tokens...)
}

func ServiceChoices(stats *usage.Stats, source string, preferences Presentation) []ServiceChoice {
	result := []ServiceChoice{}
	for _, name := range serviceNames(stats, source) {
		selection := preferences.content(source, name)
		result = append(result, ServiceChoice{ID: name, Enabled: selection.Enabled, Content: selection.Content})
	}
	return result
}

func serviceTokens(stats *usage.Stats, source, name string) []ThemeToken {
	key := name
	if source == "Local" {
		for id, alias := range localServiceNames {
			if alias == name {
				key = id
				break
			}
		}
	}
	result := []ThemeToken{}
	for _, item := range []struct {
		label  string
		period usage.Period
	}{{"Today", stats.Periods.Today}, {"Month", stats.Periods.Month}, {"All", stats.Periods.AllTime}} {
		token := ThemeToken{Label: item.label, TokensText: "—", CostText: "—", TokensFontSize: 24}
		if item.period.ClientBreakdown != nil {
			if value, ok := item.period.Clients[key]; ok {
				token.TokensText = commas(fmt.Sprint(value))
			}
			if value, ok := item.period.ClientCosts[key]; ok {
				token.CostText = usd(value)
			}
		}
		result = append(result, token)
	}
	return result
}

func serviceIcon(name string) string {
	key := strings.ToLower(name)
	if name == "Grok Build" {
		key = "grok"
	}
	if _, err := fs.Stat(iconFiles, "icons/"+key+".png"); err == nil {
		return "/assets/agents/" + key + ".png"
	}
	return ""
}

func ServicePages(stats *usage.Stats, now time.Time, config Configuration, hidden map[string]bool) ([]ServicePage, string) {
	if stats == nil {
		if config.Source == "Local" {
			return nil, "Waiting for local usage"
		}
		return nil, "Waiting for Hub"
	}
	names := serviceNames(stats, config.Source)
	if len(names) == 0 {
		return nil, "No services to display"
	}
	visible := withoutHidden(stats, hidden)
	pages := []ServicePage{}
	enabled := 0
	for _, name := range names {
		selection := config.Presentation.content(config.Source, name)
		if !selection.Enabled {
			continue
		}
		enabled++
		contracts := []usage.Provider{}
		full, has5h := true, false
		for _, provider := range visible.Limits.Providers {
			if serviceName(provider.Provider, config.Source) != name {
				continue
			}
			provider.Windows = slices.DeleteFunc(slices.Clone(provider.Windows), func(w usage.Window) bool { return !w.ShowMeter })
			for _, w := range provider.Windows {
				if minutes, ok := lengthOf(w); ok && minutes == 300 {
					has5h = true
					full = full && w.RemainingPercent != nil && *w.RemainingPercent == 100
				}
			}
			if len(provider.Windows) > 0 {
				contracts = append(contracts, provider)
			}
		}
		if config.Presentation.SkipFull5h && (len(contracts) == 0 || has5h && full) {
			continue
		}
		base := ServicePage{Provider: name, Icon: serviceIcon(name), Initial: string([]rune(name)[0]), Tokens: serviceTokens(stats, config.Source, name), Circles: []ThemeCircle{}, Windows: []ThemeWindow{}, ShowTokens: selection.Content != "Limits", ShowLimits: selection.Content != "Tokens", Both: selection.Content == "Both"}
		if !base.ShowLimits {
			base.Key = name + "\x1etokens"
			pages = append(pages, base)
			continue
		}
		low := func(p usage.Provider) float64 {
			value := math.Inf(1)
			for _, w := range p.Windows {
				if w.RemainingPercent != nil {
					value = min(value, *w.RemainingPercent)
				}
			}
			return value
		}
		slices.SortStableFunc(contracts, func(a, b usage.Provider) int { return cmp.Compare(low(a), low(b)) })
		start := len(pages)
		for _, provider := range contracts {
			contractBase := base
			contractBase.Plan = provider.PlanLabel
			if contractBase.Plan == "" {
				contractBase.Plan = provider.AccountLabel
			}
			appendPart := func(circles []ThemeCircle, windows []usage.Window) {
				page := contractBase
				page.Circles = circles
				page.Windows = []ThemeWindow{}
				key := []string{name, provider.AccountLabel, provider.PlanLabel}
				for _, w := range windows {
					page.Windows = append(page.Windows, themeWindow(w, now))
					key = append(key, windowKey(provider, w))
				}
				page.Key = strings.Join(key, "\x1e")
				pages = append(pages, page)
			}
			if config.Theme.DataLayout == "service-bars" {
				for i := 0; i < len(provider.Windows); i += 3 {
					appendPart(nil, provider.Windows[i:min(i+3, len(provider.Windows))])
				}
			} else {
				panel := panels(usage.Limits{Providers: []usage.Provider{provider}})[0]
				for i := 0; i < len(panel.circles); i += 3 {
					circles := []ThemeCircle{}
					windows := []usage.Window{}
					for _, c := range panel.circles[i:min(i+3, len(panel.circles))] {
						circle := ThemeCircle{Name: groupOf(c.windows[0]), Double: len(c.windows) == 2, ShowName: panel.groups > 1}
						for _, w := range c.windows {
							circle.Windows = append(circle.Windows, themeWindow(w, now))
							windows = append(windows, w)
						}
						circles = append(circles, circle)
					}
					appendPart(circles, windows)
				}
			}
		}
		if len(pages) == start {
			base.Key = name + "\x1eempty"
			base.Message = "No usage limits to display"
			pages = append(pages, base)
		}
	}
	if len(pages) == 0 {
		if enabled == 0 {
			return nil, "No services selected"
		}
		return nil, "All services skipped"
	}
	for i := range pages {
		pages[i].Number = i + 1
		pages[i].Count = len(pages)
	}
	return pages, ""
}
