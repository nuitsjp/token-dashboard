//go:build !windows

package localusage

import "time"

// processCreated returns the time instead, which is close to it when the machine is not busy.
func processCreated() time.Time { return time.Now() }
