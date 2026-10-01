package localusage

import (
	"time"

	"golang.org/x/sys/windows"
)

// processCreated returns when this process was created.
func processCreated() time.Time {
	var created, exited, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(windows.CurrentProcess(), &created, &exited, &kernel, &user); err != nil {
		return time.Now()
	}
	return time.Unix(0, created.Nanoseconds())
}
