package display

import (
	"maps"
	"slices"
	"strings"

	"token-monitor-turzx/internal/fault"
	"token-monitor-turzx/internal/usage"
)

// ServiceContent is the compact display choice for one service.
type ServiceContent struct {
	Provider   string `json:"provider"`
	Enabled    *bool  `json:"enabled"`
	ShowLimits bool   `json:"showLimits"`
	ShowTokens bool   `json:"showTokens"`
}

// A selection is immutable. Options retain the same pointer until a user changes it,
// so data updates keep the rotation deadline and pending frames retain their settings.
type serviceSelection struct {
	content map[string]ServiceContent
}

func contentOf(selection *serviceSelection, provider string) ServiceContent {
	content := ServiceContent{Provider: provider, ShowLimits: true, ShowTokens: true}
	if selection != nil {
		if saved, ok := selection.content[provider]; ok {
			content = saved
		}
	}
	// The existing two flags encode Off. Keep that interpretation until the
	// independent enabled flag has been saved, and retain a valid content choice.
	if content.Enabled == nil {
		enabled := content.ShowLimits || content.ShowTokens
		content.Enabled = &enabled
	}
	if !content.ShowLimits && !content.ShowTokens {
		content.ShowLimits, content.ShowTokens = true, true
	}
	content.Provider = provider
	return content
}

func serviceNames(stats *usage.Stats, source string) []string {
	var names []string
	if stats == nil {
		return names
	}
	seen := map[string]bool{}
	for _, p := range stats.Limits.Providers {
		name := serviceID(source, p.Provider)
		if name != "" && !seen[name] {
			seen[name] = true
			names = append(names, name)
		}
	}
	additional := map[string]bool{}
	for _, period := range []usage.Period{stats.Periods.Today, stats.Periods.Month, stats.Periods.AllTime} {
		if period.ClientBreakdown == nil {
			continue
		}
		for name := range period.Clients {
			name = serviceID(source, name)
			if name != "" && !seen[name] {
				additional[name] = true
			}
		}
		for name := range period.ClientCosts {
			name = serviceID(source, name)
			if name != "" && !seen[name] {
				additional[name] = true
			}
		}
	}
	return append(names, slices.Sorted(maps.Keys(additional))...)
}

// Selection returns the current immutable compact choice for the drawing/output options.
func Selection(s *Service) (*serviceSelection, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.Content != nil {
		content, err := s.Content()
		if err != nil {
			return nil, err
		}
		var before map[string]ServiceContent
		if s.selection != nil {
			before = s.selection.content
		}
		if !maps.EqualFunc(before, content, func(a, b ServiceContent) bool {
			return a.Provider == b.Provider && a.ShowLimits == b.ShowLimits && a.ShowTokens == b.ShowTokens &&
				(a.Enabled == nil && b.Enabled == nil || a.Enabled != nil && b.Enabled != nil && *a.Enabled == *b.Enabled)
		}) {
			s.selection = &serviceSelection{content: maps.Clone(content)}
		}
	}
	return s.selection, nil
}

// Services includes tools with token attribution even when no limits are reported.
func (s *Service) Services() (list []ServiceContent, err error) {
	defer func() { err = fault.Boundary(s.Logger, "display.services", err) }()
	stats, source := s.State.Snapshot()
	selection, err := Selection(s)
	if err != nil {
		return nil, err
	}
	list = make([]ServiceContent, 0)
	for _, name := range serviceNames(stats, source) {
		list = append(list, serviceContent(selection, source, name))
	}
	return list, nil
}

// SetServiceContent publishes a choice only after its persistent save succeeds.
func (s *Service) SetServiceContent(provider string, enabled, showLimits, showTokens bool) (list []ServiceContent, err error) {
	defer func() { err = fault.Boundary(s.Logger, "display.setServiceContent", err) }()
	if strings.TrimSpace(provider) == "" {
		return nil, fault.Validation(map[string]string{"provider": "Choose a service."})
	}
	if !showLimits && !showTokens {
		return nil, fault.Validation(map[string]string{"content": "Choose Both, Limits, or Tokens."})
	}
	_, source := s.State.Snapshot()
	provider = serviceID(source, provider)
	s.mu.Lock()
	if err := s.SaveContent(provider, enabled, showLimits, showTokens); err != nil {
		s.mu.Unlock()
		return nil, err
	}
	s.mu.Unlock()
	s.State.Touch()
	return s.Services()
}
