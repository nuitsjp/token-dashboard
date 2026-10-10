//go:build !windows

package desktop

import "token-monitor-turzx/internal/fault"

func chooseStyleFolder() (string, error) {
	return "", fault.New("DESKTOP_ONLY", "Folder selection is available in the desktop app.")
}
