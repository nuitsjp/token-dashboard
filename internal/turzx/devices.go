// Package turzx talks to TURZX 9.2-inch USB displays over WinUSB.
package turzx

import (
	"fmt"
	"strings"
)

// Device is a connected TURZX display.
type Device struct {
	// ID is the USB device instance ID, e.g. USB\VID_1CBE&PID_0092\633A6E01A48A0706.
	ID string
	// Name is the product string reported over USB followed by the serial prefix,
	// e.g. "TURZX1.0 (633A6E01)".
	Name string
}

const target = "vid_1cbe&pid_0092"

// deviceID converts an interface path such as
// \?\USB#VID_1CBE&PID_0092#633a6e01a48a0706#{guid} to its device instance ID.
func deviceID(path string) (string, error) {
	segments := strings.Split(path, "#")
	if len(segments) < 4 || !strings.Contains(strings.ToLower(path), target) {
		return "", fmt.Errorf("not a TURZX 9.2-inch interface path")
	}
	return strings.ToUpper(`USB\` + segments[1] + `\` + segments[2]), nil
}

func displayName(id, product string) string {
	serial := id[strings.LastIndex(id, `\`)+1:]
	if len(serial) > 8 {
		serial = serial[:8]
	}
	return fmt.Sprintf("%s (%s)", product, serial)
}
