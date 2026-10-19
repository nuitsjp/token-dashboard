package settings

import (
	"token-monitor-turzx/internal/fault"
	"token-monitor-turzx/internal/turzx"
)

// compatibleStyle chooses the agreed default only when a known output rejects the current size.
func (s *Service) compatibleStyle(saved file, devices []turzx.Device) (file, error) {
	target := targetID(saved, devices)
	profile, known := turzx.ProfileForID(target)
	current, exists := s.themes[themeID(saved.LimitStyle)]
	if !known || !exists || profile.Supports(current.Width, current.Height) {
		return saved, nil
	}
	id := "gauges"
	if profile.Model == "turzx-35-reva" {
		id = "gauges-35-landscape"
	}
	theme, exists := s.themes[id]
	if !exists || !profile.Supports(theme.Width, theme.Height) {
		return saved, fault.New("STYLE_UNAVAILABLE", "The default style for this output is unavailable.")
	}
	saved.LimitStyle = id
	return saved, nil
}

// reconcileStyle commits a correction before changing the generation or notifying rendering.
// The caller holds the settings lock.
func (s *Service) reconcileStyle(saved file, devices []turzx.Device) (file, error) {
	next, err := s.compatibleStyle(saved, devices)
	if err != nil || next.LimitStyle == saved.LimitStyle {
		return saved, err
	}
	if err := s.write(next); err != nil {
		return saved, err
	}
	s.generation++
	if s.OnDisplaySaved != nil {
		s.OnDisplaySaved()
	}
	return next, nil
}
