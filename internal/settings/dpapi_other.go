//go:build !windows

package settings

import "errors"

var errUnsupported = errors.New("DPAPI is available only on Windows")

func protect(_, _ []byte) ([]byte, error)   { return nil, errUnsupported }
func unprotect(_, _ []byte) ([]byte, error) { return nil, errUnsupported }
