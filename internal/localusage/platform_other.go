//go:build !windows

package localusage

import (
	"context"
	"errors"
	"os/exec"
)

func hideWindow(*exec.Cmd) {}

func watch(context.Context, string, func()) error {
	return errors.New("watching local usage is supported only on Windows")
}
