//go:build windows

package turzx

import (
	"errors"
	"fmt"
	"syscall"
	"time"
	"unsafe"
)

var (
	winusb                  = syscall.NewLazyDLL("winusb.dll")
	procInitialize          = winusb.NewProc("WinUsb_Initialize")
	procFree                = winusb.NewProc("WinUsb_Free")
	procQueryInterfaceSetts = winusb.NewProc("WinUsb_QueryInterfaceSettings")
	procQueryPipe           = winusb.NewProc("WinUsb_QueryPipe")
	procSetPipePolicy       = winusb.NewProc("WinUsb_SetPipePolicy")
	procReadPipe            = winusb.NewProc("WinUsb_ReadPipe")
	procWritePipe           = winusb.NewProc("WinUsb_WritePipe")
)

const (
	pipeTransferTimeout = 3
	usbdPipeBulk        = 2
	errorSemTimeout     = syscall.Errno(121)
)

// USB_INTERFACE_DESCRIPTOR
type interfaceDescriptor struct {
	Length, DescriptorType, InterfaceNumber, AlternateSetting, NumEndpoints, Class, SubClass, Protocol, Interface uint8
}

// WINUSB_PIPE_INFORMATION
type pipeInformation struct {
	PipeType          int32
	PipeID            uint8
	MaximumPacketSize uint16
	Interval          uint8
}

// Conn is an open connection to one TURZX display.
type Conn struct {
	file    syscall.Handle
	usb     uintptr
	in, out uint8
}

// Open opens the display whose device instance ID (Device.ID from List) is id,
// and performs the sync command.
func Open(id string) (*Conn, error) {
	paths, err := interfacePaths()
	if err != nil {
		return nil, err
	}
	for _, path := range paths {
		if pathID, err := deviceID(path); err == nil && pathID == id {
			return openPath(path)
		}
	}
	return nil, fmt.Errorf("TURZX display %s not found", id)
}

func openPath(path string) (*Conn, error) {
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return nil, err
	}
	file, err := syscall.CreateFile(name, syscall.GENERIC_READ|syscall.GENERIC_WRITE,
		syscall.FILE_SHARE_READ|syscall.FILE_SHARE_WRITE, nil, syscall.OPEN_EXISTING, syscall.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", path, err)
	}
	c := &Conn{file: file}
	if err := c.init(); err != nil {
		c.Close()
		return nil, err
	}
	return c, nil
}

func (c *Conn) init() error {
	if r, _, err := procInitialize.Call(uintptr(c.file), uintptr(unsafe.Pointer(&c.usb))); r == 0 {
		return fmt.Errorf("WinUsb_Initialize: %w", err)
	}
	var descriptor interfaceDescriptor
	if r, _, err := procQueryInterfaceSetts.Call(c.usb, 0, uintptr(unsafe.Pointer(&descriptor))); r == 0 {
		return fmt.Errorf("WinUsb_QueryInterfaceSettings: %w", err)
	}
	for i := range descriptor.NumEndpoints {
		var pipe pipeInformation
		if r, _, err := procQueryPipe.Call(c.usb, 0, uintptr(i), uintptr(unsafe.Pointer(&pipe))); r == 0 {
			return fmt.Errorf("WinUsb_QueryPipe: %w", err)
		}
		if pipe.PipeType != usbdPipeBulk {
			continue
		}
		if pipe.PipeID&0x80 != 0 {
			c.in = pipe.PipeID
		} else {
			c.out = pipe.PipeID
		}
		if err := c.setTimeout(pipe.PipeID, 2*time.Second); err != nil {
			return err
		}
	}
	if c.in == 0 || c.out == 0 {
		return errors.New("TURZX bulk IN/OUT endpoints not found")
	}
	if err := c.drain(); err != nil {
		return err
	}
	return c.exchange(cmdSync, nil)
}

// SendJPEG sends one 462x1920 baseline JPEG (<= 1 MiB) and waits for the
// device's success response.
func (c *Conn) SendJPEG(data []byte) error {
	if err := validateJPEG(data); err != nil {
		return err
	}
	return c.exchange(cmdJPEG, data)
}

// Restart asks the display to restart (command 11). It does not wait for a
// response because the device may disconnect.
func (c *Conn) Restart() error {
	p, _ := packet(cmdRestart, nil, time.Now())
	return c.write(p)
}

func (c *Conn) Close() error {
	if c.usb != 0 {
		procFree.Call(c.usb)
		c.usb = 0
	}
	if c.file == 0 {
		return nil
	}
	err := syscall.CloseHandle(c.file)
	c.file = 0
	return err
}

func (c *Conn) exchange(command byte, payload []byte) error {
	p, timestamp := packet(command, payload, time.Now())
	if err := c.write(p); err != nil {
		return err
	}
	response, err := c.read()
	if err != nil {
		return err
	}
	return checkResponse(command, timestamp, response)
}

func (c *Conn) write(p []byte) error {
	var written uint32
	if r, _, err := procWritePipe.Call(c.usb, uintptr(c.out), uintptr(unsafe.Pointer(&p[0])), uintptr(len(p)), uintptr(unsafe.Pointer(&written)), 0); r == 0 {
		return fmt.Errorf("WinUsb_WritePipe: %w", err)
	}
	if int(written) != len(p) {
		return fmt.Errorf("USB short write: %d/%d bytes", written, len(p))
	}
	return nil
}

// read returns one response, skipping the zero-length packet that can follow a
// 512-byte response.
func (c *Conn) read() ([]byte, error) {
	buffer := make([]byte, 512)
	for {
		var n uint32
		if r, _, err := procReadPipe.Call(c.usb, uintptr(c.in), uintptr(unsafe.Pointer(&buffer[0])), uintptr(len(buffer)), uintptr(unsafe.Pointer(&n)), 0); r == 0 {
			return nil, fmt.Errorf("WinUsb_ReadPipe: %w", err)
		}
		if n > 0 {
			return buffer[:n], nil
		}
	}
}

// drain discards stale responses left in the IN pipe, e.g. by a previous process.
func (c *Conn) drain() error {
	if err := c.setTimeout(c.in, 100*time.Millisecond); err != nil {
		return err
	}
	for range 16 {
		if _, err := c.read(); err != nil {
			if !errors.Is(err, errorSemTimeout) {
				return fmt.Errorf("drain USB input: %w", err)
			}
			return c.setTimeout(c.in, 2*time.Second)
		}
	}
	return errors.New("USB input remained busy")
}

func (c *Conn) setTimeout(pipe uint8, timeout time.Duration) error {
	value := uint32(timeout.Milliseconds())
	if r, _, err := procSetPipePolicy.Call(c.usb, uintptr(pipe), pipeTransferTimeout, 4, uintptr(unsafe.Pointer(&value))); r == 0 {
		return fmt.Errorf("WinUsb_SetPipePolicy: %w", err)
	}
	return nil
}
