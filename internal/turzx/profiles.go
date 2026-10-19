package turzx

import "strings"

// RenderSize is the upright drawing size, before transport rotation or encoding.
type RenderSize struct{ Width, Height int }

// Profile describes device capabilities independently of display style names.
type Profile struct {
	Model       string
	Transport   string
	RenderSizes []RenderSize
}

// ProfileForID also identifies a saved, disconnected device without opening it.
func ProfileForID(id string) (Profile, bool) {
	parts := strings.Split(strings.ToUpper(id), `\`)
	if len(parts) != 3 || parts[0] != "USB" {
		return Profile{}, false
	}
	hardware := strings.Split(parts[1], "&")
	if len(hardware) < 2 {
		return Profile{}, false
	}
	if hardware[0] == "VID_1A86" && hardware[1] == "PID_5722" && parts[2] == "USB35INCHIPSV2" {
		return Profile{Model: "turzx-35-reva", Transport: "serial-reva", RenderSizes: []RenderSize{{480, 320}, {320, 480}}}, true
	}
	if hardware[0] == "VID_1CBE" && hardware[1] == "PID_0092" && parts[2] != "" {
		return Profile{Model: "turzx-92", Transport: "winusb", RenderSizes: []RenderSize{{1920, 462}}}, true
	}
	return Profile{}, false
}

func (p Profile) Supports(width, height int) bool {
	for _, size := range p.RenderSizes {
		if size.Width == width && size.Height == height {
			return true
		}
	}
	return false
}
