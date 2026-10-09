//go:build windows

package turzx

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// The COM interface path is resolved anew by Open, even when Windows changes its COM number.
func openSerial(path string) (*Conn, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	file, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("open serial interface: %w", err)
	}
	c := &Conn{file: file, serial: true}
	state := windows.DCB{DCBlength: uint32(unsafe.Sizeof(windows.DCB{}))}
	if err = windows.GetCommState(windows.Handle(file), &state); err == nil {
		state.BaudRate, state.ByteSize, state.Parity, state.StopBits = 115200, 8, windows.NOPARITY, windows.ONESTOPBIT
		// Binary, CTS flow control, DTR enabled, RTS handshake; software flow control off.
		state.Flags = 1 | 4 | windows.DTR_CONTROL_ENABLE | windows.RTS_CONTROL_HANDSHAKE
		err = windows.SetCommState(windows.Handle(file), &state)
	}
	if err == nil {
		err = windows.SetCommTimeouts(windows.Handle(file), &windows.CommTimeouts{ReadIntervalTimeout: 0xffffffff, ReadTotalTimeoutConstant: 1000, WriteTotalTimeoutConstant: 2000})
	}
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("configure serial interface: %w", err)
	}
	if err = c.serialWrite(serialCommand(109, 0, 0)); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func (c *Conn) serialWrite(data []byte) error {
	var written uint32
	if err := syscall.WriteFile(c.file, data, &written, nil); err != nil {
		return fmt.Errorf("serial write: %w", err)
	}
	if int(written) != len(data) {
		return fmt.Errorf("serial short write: %d/%d", written, len(data))
	}
	return nil
}

// SendRGB565 configures orientation before sending one whole compact frame sequentially.
func (c *Conn) SendRGB565(data []byte, orientation string) error {
	if !c.serial {
		return fmt.Errorf("RGB565 requires a compact serial display")
	}
	command, w, h, err := serialOrientation(orientation)
	if err != nil {
		return err
	}
	if len(data) != w*h*2 {
		return fmt.Errorf("RGB565 must contain %d bytes, got %d", w*h*2, len(data))
	}
	if c.orientation != orientation {
		if err := c.serialWrite(command); err != nil {
			return err
		}
		c.orientation = orientation
	}
	if err := c.serialWrite(serialCommand(197, w-1, h-1)); err != nil {
		return err
	}
	for start := 0; start < len(data); start += w * 8 {
		if err := c.serialWrite(data[start:min(start+w*8, len(data))]); err != nil {
			return err
		}
	}
	return nil
}
