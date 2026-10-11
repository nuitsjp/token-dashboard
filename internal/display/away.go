package display

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"sync"

	"token-monitor-turzx/internal/fault"
)

// Away minutes the user may choose, and the default.
const (
	MinAwayMinutes     = 1
	MaxAwayMinutes     = 240
	DefaultAwayMinutes = 10
)

var (
	blackOnce sync.Once
	blackJPEG []byte
)

// black is the all-black image, in the orientation the TURZX takes, that switches the display off.
func black() []byte {
	blackOnce.Do(func() {
		var buf bytes.Buffer
		_ = jpeg.Encode(&buf, image.NewGray(image.Rect(0, 0, Height, Width)), &jpeg.Options{Quality: 85})
		blackJPEG = buf.Bytes()
	})
	return blackJPEG
}

// SetAway tells that the user is away, so the TURZX is switched off, or back. It is a function, not a
// method, so Wails does not bind it.
func SetAway(s *Service, away bool) {
	s.mu.Lock()
	s.away = away
	if s.awayCh == nil {
		s.awayCh = make(chan struct{}, 1)
	}
	ch := s.awayCh
	s.mu.Unlock()
	select {
	case ch <- struct{}{}:
	default:
	}
}

func (s *Service) awayState() (bool, <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.awayCh == nil {
		s.awayCh = make(chan struct{}, 1)
	}
	return s.away, s.awayCh
}

// AwayMinutes is how long without input counts as away.
func (s *Service) AwayMinutes() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.awayMinutes == 0 {
		return DefaultAwayMinutes
	}
	return s.awayMinutes
}

// SetAwayMinutes changes the time without input that counts as away; a value out of range is refused.
func (s *Service) SetAwayMinutes(minutes int) (saved int, err error) {
	defer func() { err = fault.Boundary(s.Logger, "display.setAwayMinutes", err) }()
	if minutes < MinAwayMinutes || minutes > MaxAwayMinutes {
		return s.AwayMinutes(), fmt.Errorf("enter %d to %d minutes", MinAwayMinutes, MaxAwayMinutes)
	}
	s.mu.Lock()
	s.awayMinutes = minutes
	s.mu.Unlock()
	return minutes, nil
}
