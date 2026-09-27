package turzx

import "testing"

func TestDeviceID(t *testing.T) {
	id, err := deviceID(`\\?\USB#VID_1CBE&PID_0092#633a6e01a48a0706#{a5dcbf10-6530-11d2-901f-00c04fb951ed}`)
	if err != nil || id != `USB\VID_1CBE&PID_0092\633A6E01A48A0706` {
		t.Fatalf("deviceID = %q, %v", id, err)
	}
	if _, err := deviceID(`\\?\USB#VID_1234&PID_5678#abc#{a5dcbf10-6530-11d2-901f-00c04fb951ed}`); err == nil {
		t.Fatal("accepted a non-TURZX path")
	}
}

func TestDisplayName(t *testing.T) {
	if got := displayName(`USB\VID_1CBE&PID_0092\633A6E01A48A0706`, "TURZX1.0"); got != "TURZX1.0 (633A6E01)" {
		t.Fatalf("displayName = %q", got)
	}
}
