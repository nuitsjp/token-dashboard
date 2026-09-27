//go:build windows

package turzx

import (
	"os"
	"regexp"
	"testing"
)

// Run with TURZX_DEVICE_TEST=1 on a PC with a TURZX connected.
func TestListConnected(t *testing.T) {
	if os.Getenv("TURZX_DEVICE_TEST") != "1" {
		t.Skip("set TURZX_DEVICE_TEST=1 with a TURZX connected")
	}
	devices, err := List()
	if err != nil || len(devices) == 0 {
		t.Fatalf("List = %v, %v", devices, err)
	}
	name := regexp.MustCompile(`^.+ \([0-9A-F]{8}\)$`)
	for _, d := range devices {
		if !name.MatchString(d.Name) {
			t.Errorf("unexpected name %q", d.Name)
		}
	}
	t.Logf("%+v", devices)
}
