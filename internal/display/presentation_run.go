package display

import (
	"context"
	"encoding/base64"
	"reflect"
	"time"
)

// Services supplies the same service identities and selections used by the renderer.
func (s *Service) Services() ([]ServiceChoice, error) {
	stats, source := s.State.Snapshot()
	preferences := Presentation{}
	if s.Configuration != nil {
		config, err := s.Configuration()
		if err != nil {
			return nil, err
		}
		preferences = config.Presentation
	}
	return ServiceChoices(stats, source, preferences), nil
}

// ThemePreview prepares an independent first page; it never changes the resident cursor.
func (s *Service) ThemePreview(id string) (ThemeData, error) {
	stats, source := s.State.Snapshot()
	hidden, err := s.hiddenSet()
	if err != nil {
		return ThemeData{}, err
	}
	config := Configuration{Source: source, Theme: s.Themes[id]}
	if s.Theme != nil {
		config.Theme, err = s.Theme(id)
		if err != nil {
			return ThemeData{}, err
		}
	}
	if s.Configuration != nil {
		selected, err := s.Configuration()
		if err != nil {
			return ThemeData{}, err
		}
		config.Presentation = selected.Presentation
	}
	data := themeData(withoutHidden(stats, hidden), time.Now(), source, 0)
	if config.Theme.ServiceLayout() {
		pages, message := ServicePages(stats, time.Now(), config, hidden)
		data.ServiceMessage = message
		if len(pages) > 0 {
			data.Service = &pages[0]
		}
	}
	return data, nil
}

// RunPresentation shares one page scheduler and Canvas path for every selected style.
func RunPresentation(ctx context.Context, s *Service, renderer *CanvasRenderer, redraw time.Duration, output func(Frame), emit func(string, any)) {
	runPresentation(ctx, s, renderer, redraw, s.Configuration, output, emit)
}

func runPresentation(ctx context.Context, s *Service, renderer *CanvasRenderer, redraw time.Duration, configuration func() (Configuration, error), output func(Frame), emit func(string, any)) {
	beat := time.NewTicker(redraw)
	defer beat.Stop()
	poll := time.NewTicker(2 * time.Second)
	defer poll.Stop()
	shifter := time.NewTicker(s.shiftEvery())
	defer shifter.Stop()
	step := 0
	wasAway := false
	var previous Configuration
	var oldPages []ServicePage
	page := 0
	deadline := time.Now()
	initialized := false
	hadStats := false
	for {
		config, err := configuration()
		if err != nil {
			s.Logger.Error("display_configuration_failed", "cause", err)
			select {
			case <-ctx.Done():
				return
			case <-s.State.Changed():
			case <-poll.C:
			}
			continue
		}
		now := time.Now()
		away, awayChanged := s.awayState()
		count := 0
		if away {
			frame, blackErr := blackFrame(presentationRequest(config, step))
			frame.presence = awayChanged
			if blackErr != nil {
				s.Logger.Error("display_black_failed", "cause", blackErr)
			} else {
				output(frame)
			}
			wasAway = true
		} else {
			previousPage := page
			stats, source := s.State.Snapshot()
			if config.Source != source {
				select {
				case <-ctx.Done():
					return
				case <-s.State.Changed():
				case <-awayChanged:
				case <-poll.C:
				}
				continue
			}
			hidden, err := s.hiddenSet()
			if err != nil {
				s.Logger.Warn("hidden_limits_unavailable", "cause", err)
			}
			data := themeData(withoutHidden(stats, hidden), now, source, page)
			pages := []ServicePage{}
			count = data.GaugePages
			if config.Theme.DataLayout == "bars" {
				count = data.BarPages
			}
			reset := !initialized || stats != nil && !hadStats || (config.Theme.ID != previous.Theme.ID && (config.Theme.ServiceLayout() || previous.Theme.ServiceLayout())) || config.Source != previous.Source || config.Cycle != previous.Cycle || config.Theme.ServiceLayout() && (!reflect.DeepEqual(config.Presentation.Services, previous.Presentation.Services) || config.Presentation.SkipFull5h != previous.Presentation.SkipFull5h)
			if config.Theme.ServiceLayout() {
				var message string
				pages, message = ServicePages(stats, now, config, hidden)
				data.ServiceMessage = message
				count = len(pages)
				if !reset && config.Cycle && len(oldPages) > 0 {
					found := false
					for offset := 0; offset < len(oldPages); offset++ {
						key := oldPages[(page+offset)%len(oldPages)].Key
						for index, candidate := range pages {
							if candidate.Key == key {
								page = index
								found = true
								break
							}
						}
						if found {
							break
						}
					}
					if !found {
						page = 0
					}
				}
			}
			interval := config.CycleEvery
			if interval <= 0 {
				interval = time.Duration(config.CycleSeconds) * time.Second
			}
			if interval <= 0 {
				interval = 10 * time.Second
			}
			if reset || !config.Cycle || page >= count {
				page = 0
			}
			if reset || wasAway || config.CycleSeconds != previous.CycleSeconds || config.CycleEvery != previous.CycleEvery {
				deadline = now.Add(interval)
			}
			if config.Cycle && count > 1 && !away && !wasAway && !now.Before(deadline) {
				page = (page + 1) % count
				deadline = now.Add(interval)
			}
			if len(pages) > 0 {
				data.Service = &pages[page]
			} else if !config.Theme.ServiceLayout() {
				data = themeData(withoutHidden(stats, hidden), now, source, page)
			}
			request := presentationRequest(config, step)
			request.Data = data
			if wasAway && !away {
				step = 0
				shifter.Reset(s.shiftEvery())
				request.ShiftX, request.ShiftY = 0, 0
			}
			frame, err := renderer.RenderPrepared(ctx, request, count)
			if ctx.Err() != nil {
				return
			}
			if err == errPresenceChanged || !frame.presenceCurrent() {
				page = previousPage
				continue
			}
			if err != nil {
				s.Logger.Error("display_render_failed", "cause", err)
			} else {
				s.mu.Lock()
				current := s.awayCh == awayChanged
				if current {
					s.preview = "data:image/png;base64," + base64.StdEncoding.EncodeToString(frame.PNG)
				}
				s.mu.Unlock()
				if !current {
					page = previousPage
					continue
				}
				output(frame)
				emit(Updated, nil)
				if wasAway {
					deadline = time.Now().Add(interval)
				}
				wasAway = false
			}
			previous = config
			oldPages = pages
			initialized = true
			hadStats = stats != nil
		}
		delay := time.Hour
		if config.Cycle && count > 1 && !away {
			delay = max(time.Millisecond, time.Until(deadline))
		}
		timer := time.NewTimer(delay)
	wait:
		for {
			select {
			case <-ctx.Done():
				timer.Stop()
				return
			case <-s.State.Changed():
				break wait
			case <-beat.C:
				break wait
			case <-timer.C:
				break wait
			case <-awayChanged:
				break wait
			case <-shifter.C:
				if !away {
					step = (step + 1) % len(shiftSteps)
					break wait
				}
			case <-poll.C:
				next, configErr := configuration()
				if configErr != nil || next.Generation != config.Generation || next.TargetID != config.TargetID {
					break wait
				}
			}
		}
		timer.Stop()
	}
}

// presentationRequest uses the same metadata for normal and black device images.
func presentationRequest(config Configuration, step int) FrameRequest {
	request := FrameRequest{TargetID: config.TargetID, Theme: config.Theme.ID, Width: config.Theme.Width, Height: config.Theme.Height, OutputFormat: "png", Reverse180: config.Presentation.Reverse180, Generation: config.Generation}
	request.ShiftX, request.ShiftY = shiftSteps[step][0], shiftSteps[step][1]
	if config.Theme.Width == Width && config.Theme.Height == Height {
		request.OutputFormat = "jpeg"
		request.Reverse180 = false
	}
	return request
}
