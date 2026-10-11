package display

import (
	"bytes"
	"image"
	"image/jpeg"
	"sync"
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
