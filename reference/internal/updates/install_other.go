//go:build !windows || server

package updates

import "errors"

func LaunchInstaller(string) error {
	return errors.New("installer execution requires a Windows desktop build")
}
