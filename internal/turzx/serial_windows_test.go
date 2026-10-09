//go:build windows

package turzx

import (
	"image"
	"image/draw"
	"image/png"
	"os"
	"testing"
	"time"
)

// Run explicitly with the desktop app stopped. The PNG is a current preview captured outside
// the repository; the test sends it, resets the display, resolves its interface again and resends it.
func TestSerialReconnectConnectedCompactDisplay(t *testing.T) {
	if os.Getenv("TURZX_SERIAL_TEST") != "1" {
		t.Skip("set TURZX_SERIAL_TEST=1 with the desktop app stopped")
	}
	f, err := os.Open(os.Getenv("TURZX_FRAME_FILE"))
	if err != nil {
		t.Fatal(err)
	}
	img, err := png.Decode(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	rgba := image.NewRGBA(image.Rect(0, 0, img.Bounds().Dx(), img.Bounds().Dy()))
	draw.Draw(rgba, rgba.Bounds(), img, img.Bounds().Min, draw.Src)
	data := RGB565(rgba)
	orientation := os.Getenv("TURZX_FRAME_ORIENTATION")
	devices, err := List()
	if err != nil {
		t.Fatal(err)
	}
	var id string
	for _, device := range devices {
		if IsCompact(device.ID) {
			id = device.ID
			break
		}
	}
	if id == "" {
		t.Fatal("no supported compact display connected")
	}
	c, err := Open(id)
	if err != nil {
		t.Fatal(err)
	}
	if err := c.SendRGB565(data, orientation); err != nil {
		c.Close()
		t.Fatal(err)
	}
	if err := c.Restart(); err != nil {
		c.Close()
		t.Fatal(err)
	}
	c.Close()
	time.Sleep(3 * time.Second)
	deadline := time.Now().Add(12 * time.Second)
	for {
		c, err = Open(id) // Resolve the current COM interface from the saved USB identity again.
		if err == nil {
			err = c.SendRGB565(data, orientation)
			c.Close()
			if err == nil {
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatalf("reconnection: %v", err)
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Logf("reconnected %s; sent %dx%d %s", id, rgba.Bounds().Dx(), rgba.Bounds().Dy(), orientation)
}
