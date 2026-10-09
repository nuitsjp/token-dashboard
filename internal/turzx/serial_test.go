package turzx

import (
	"bytes"
	"image"
	"image/color"
	"testing"
)

func TestRevAOrientationAndFullFrameCoordinates(t *testing.T) {
	for _, tc := range []struct {
		orientation string
		direction   byte
		w, h        int
		coordinates []byte
	}{
		{"Landscape", 102, 480, 320, []byte{0, 0, 7, 125, 63, 197}},
		{"ReverseLandscape", 103, 480, 320, []byte{0, 0, 7, 125, 63, 197}},
		{"Portrait", 100, 320, 480, []byte{0, 0, 4, 253, 223, 197}},
		{"ReversePortrait", 101, 320, 480, []byte{0, 0, 4, 253, 223, 197}},
	} {
		t.Run(tc.orientation, func(t *testing.T) {
			got, w, h, err := serialOrientation(tc.orientation)
			want := []byte{0, 0, 0, 0, 0, 121, tc.direction, byte(tc.w >> 8), byte(tc.w), byte(tc.h >> 8), byte(tc.h), 0, 0, 0, 0, 0}
			if err != nil || w != tc.w || h != tc.h || !bytes.Equal(got, want) {
				t.Fatalf("orientation: %x, %dx%d, %v", got, w, h, err)
			}
			if got := serialCommand(197, w-1, h-1); !bytes.Equal(got, tc.coordinates) {
				t.Fatalf("full frame bounds: %x, want %x", got, tc.coordinates)
			}
		})
	}
	if _, _, _, err := serialOrientation("other"); err == nil {
		t.Fatal("accepted an unknown orientation")
	}
}

func TestRGB565IsLittleEndianInRowOrder(t *testing.T) {
	img := image.NewRGBA(image.Rect(10, 20, 12, 22))
	for i, c := range []color.RGBA{{255, 0, 0, 255}, {0, 255, 0, 255}, {0, 0, 255, 255}, {255, 255, 255, 255}} {
		img.SetRGBA(10+i%2, 20+i/2, c)
	}
	if got, want := RGB565(img), []byte{0x00, 0xf8, 0xe0, 0x07, 0x1f, 0x00, 0xff, 0xff}; !bytes.Equal(got, want) {
		t.Fatalf("RGB565: %x, want %x", got, want)
	}
}

func TestCompactCOMInterfaceKeepsUSBIdentity(t *testing.T) {
	const id = `USB\VID_1A86&PID_5722\USB35INCHIPSV2`
	for _, path := range []string{`\\?\USB#VID_1A86&PID_5722#USB35INCHIPSV2#{86e0d1e0-8089-11d0-9ce4-08003e301f73}`, `\\?\usb#vid_1a86&pid_5722#usb35inchipsv2#{86e0d1e0-8089-11d0-9ce4-08003e301f73}`} {
		if got, err := deviceID(path); err != nil || got != id || !IsCompact(got) {
			t.Fatalf("COM identity: %q, %v", got, err)
		}
	}
	if IsCompact(`USB\VID_1A86&PID_5722\USB5INCH`) || IsCompact(`USB\VID_1CBE&PID_0092\USB35INCHIPSV2`) {
		t.Fatal("accepted another model")
	}
	if got := displayName(id, "USB Serial"); got != "TURZX 3.5-inch (USB35INC)" {
		t.Fatalf("compact name: %q", got)
	}
}
