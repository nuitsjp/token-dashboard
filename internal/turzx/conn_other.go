//go:build !windows

package turzx

import "errors"

// Conn is an open connection to one TURZX display.
type Conn struct{}

// Open is not supported outside Windows.
func Open(id string) (*Conn, error) {
	return nil, errors.New("TURZX displays are supported only on Windows")
}

// SendJPEG sends one 462x1920 baseline JPEG (<= 1 MiB).
func (c *Conn) SendJPEG(data []byte) error { return errors.ErrUnsupported }

// Restart asks the display to restart.
func (c *Conn) Restart() error { return errors.ErrUnsupported }

func (c *Conn) Close() error { return nil }
