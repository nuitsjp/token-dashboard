//go:build windows

package turzx

import (
	"fmt"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Resolve the present COM interface from the USB instance ID on every open.
// GUID_DEVINTERFACE_COMPORT is independent of the assigned COM number.
func openSerial(id string) (*Conn, error) {
	class := syscall.GUID{Data1: 0x86e0d1e0, Data2: 0x8089, Data3: 0x11d0, Data4: [8]byte{0x9c, 0xe4, 0x08, 0x00, 0x3e, 0x30, 0x1f, 0x73}}
	name, err := syscall.UTF16PtrFromString(id)
	if err != nil {
		return nil, err
	}
	paths, err := deviceInterfacePaths(&class, name)
	if err != nil {
		return nil, err
	}
	if len(paths) != 1 {
		return nil, fmt.Errorf("TURZX serial interface not available")
	}
	path, err := syscall.UTF16PtrFromString(paths[0])
	if err != nil {
		return nil, err
	}
	handle, err := syscall.CreateFile(path, syscall.GENERIC_READ|syscall.GENERIC_WRITE, 0, nil, syscall.OPEN_EXISTING, 0, 0)
	if err != nil {
		return nil, fmt.Errorf("open TURZX serial interface: %w", err)
	}
	c := &Conn{file: handle, serial: true}
	fail := func(err error) (*Conn, error) { c.Close(); return nil, err }
	var dcb windows.DCB
	dcb.DCBlength = uint32(unsafe.Sizeof(dcb))
	if err := windows.GetCommState(windows.Handle(handle), &dcb); err != nil {
		return fail(err)
	}
	dcb.BaudRate, dcb.ByteSize, dcb.Parity, dcb.StopBits = 115200, 8, 0, 0
	// Binary, CTS flow control, DTR enabled, RTS handshake (115200 8N1).
	dcb.Flags = 1 | 1<<3 | 1<<4 | 2<<12
	if err := windows.SetCommState(windows.Handle(handle), &dcb); err != nil {
		return fail(err)
	}
	if err := windows.SetCommTimeouts(windows.Handle(handle), &windows.CommTimeouts{ReadIntervalTimeout: ^uint32(0), ReadTotalTimeoutConstant: 100, WriteTotalTimeoutConstant: 2000}); err != nil {
		return fail(err)
	}
	if err := c.serialWrite(revACommand(109, 0, 0, 0, 0)); err != nil {
		return fail(err)
	}
	if err := c.serialWrite(revACommand(110, 0, 0, 0, 0)); err != nil {
		return fail(err)
	}
	return c, nil
}

func (c *Conn) serialWrite(data []byte) error {
	var n uint32
	if err := syscall.WriteFile(c.file, data, &n, nil); err != nil {
		return err
	}
	if int(n) != len(data) {
		return fmt.Errorf("serial short write: %d/%d bytes", n, len(data))
	}
	return nil
}

func (c *Conn) SendRGB565(data []byte, width, height int, reverse180 bool) error {
	if !c.serial || (width != 480 || height != 320) && (width != 320 || height != 480) || len(data) != width*height*2 {
		return fmt.Errorf("invalid TURZX RGB565 frame")
	}
	if err := c.serialWrite(revAOrientation(width, height, reverse180)); err != nil {
		return err
	}
	if err := c.serialWrite(revACommand(197, 0, 0, width-1, height-1)); err != nil {
		return err
	}
	for start := 0; start < len(data); start += width * 8 {
		if err := c.serialWrite(data[start:min(start+width*8, len(data))]); err != nil {
			return err
		}
	}
	return nil
}
