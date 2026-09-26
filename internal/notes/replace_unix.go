//go:build !windows

package notes

import "os"

func replaceFile(source, destination string) error { return os.Rename(source, destination) }
