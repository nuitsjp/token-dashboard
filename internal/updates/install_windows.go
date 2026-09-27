//go:build windows && !server

package updates

import (
	"os"
	"os/exec"
	"strconv"
)

func LaunchInstaller(path string) error {
	// No shell or user-controlled command string. The NSIS script waits for
	// this exact PID and aborts rather than killing it on timeout.
	cmd := exec.Command(path, "/S", "/UPDATEPID="+strconv.Itoa(os.Getpid()), "/RESTART=1")
	if err := cmd.Start(); err != nil {
		return err
	}
	// The parent exits shortly. Reaping in the normal case avoids a handle leak.
	go cmd.Wait()
	return nil
}
