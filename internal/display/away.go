package display

import (
	"bytes"
	"image"
	"image/jpeg"
	"image/png"
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

// blackFrame bypasses Canvas and uses the selected style's device encoding.
func blackFrame(request FrameRequest) (Frame, error) {
	frame := Frame{Request: request, OutputFormat: request.OutputFormat, away: true}
	if request.OutputFormat == "jpeg" {
		frame.DeviceImage, frame.JPEG = black(), black()
		return frame, nil
	}
	var buf bytes.Buffer
	err := png.Encode(&buf, image.NewGray(image.Rect(0, 0, request.Width, request.Height)))
	frame.DeviceImage = buf.Bytes()
	return frame, err
}

// SetAway tells that the user is away, so the TURZX is switched off, or back. It is a function, not a
// method, so Wails does not bind it.
func SetAway(s *Service, away bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.away == away {
		return
	}
	s.away = away
	if s.awayCh != nil {
		close(s.awayCh)
	}
	s.awayCh = make(chan struct{})
}

func (s *Service) awayState() (bool, <-chan struct{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.awayCh == nil {
		s.awayCh = make(chan struct{})
	}
	return s.away, s.awayCh
}
