//go:build windows

package turzx

import (
	"fmt"
	"slices"
	"strings"
	"syscall"
	"unsafe"
)

var (
	cfgmgr32               = syscall.NewLazyDLL("cfgmgr32.dll")
	procListSize           = cfgmgr32.NewProc("CM_Get_Device_Interface_List_SizeW")
	procList               = cfgmgr32.NewProc("CM_Get_Device_Interface_ListW")
	procLocateDevNode      = cfgmgr32.NewProc("CM_Locate_DevNodeW")
	procGetDevNodeProperty = cfgmgr32.NewProc("CM_Get_DevNode_PropertyW")
)

// GUID_DEVINTERFACE_USB_DEVICE is exposed by every USB device, independent of
// the GUID chosen when the WinUSB driver was installed.
var usbDeviceInterface = syscall.GUID{Data1: 0xa5dcbf10, Data2: 0x6530, Data3: 0x11d2, Data4: [8]byte{0x90, 0x1f, 0x00, 0xc0, 0x4f, 0xb9, 0x51, 0xed}}

// DEVPKEY_Device_BusReportedDeviceDesc: the product string the device reports over USB.
var busReportedDeviceDesc = struct {
	fmtid syscall.GUID
	pid   uint32
}{syscall.GUID{Data1: 0x540b947e, Data2: 0x8b40, Data3: 0x45bc, Data4: [8]byte{0xa8, 0xa2, 0x6a, 0x0b, 0x89, 0x4c, 0xbd, 0xa2}}, 4}

const (
	crSuccess     = 0
	crBufferSmall = 0x1A
)

// List returns the connected TURZX displays sorted by device ID.
func List() ([]Device, error) {
	paths, err := interfacePaths()
	if err != nil {
		return nil, err
	}
	devices := []Device{}
	for _, path := range paths {
		id, err := deviceID(path)
		if err != nil {
			continue
		}
		if slices.ContainsFunc(devices, func(d Device) bool { return d.ID == id }) {
			continue
		}
		product, err := productName(id)
		if err != nil {
			return nil, err
		}
		devices = append(devices, Device{ID: id, Name: displayName(id, product)})
	}
	slices.SortFunc(devices, func(a, b Device) int { return strings.Compare(a.ID, b.ID) })
	return devices, nil
}

func interfacePaths() ([]string, error) {
	for {
		var size uint32
		if r, _, _ := procListSize.Call(uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&usbDeviceInterface)), 0, 0); r != crSuccess {
			return nil, fmt.Errorf("CM_Get_Device_Interface_List_SizeW: CONFIGRET=0x%X", r)
		}
		buffer := make([]uint16, size)
		r, _, _ := procList.Call(uintptr(unsafe.Pointer(&usbDeviceInterface)), 0, uintptr(unsafe.Pointer(&buffer[0])), uintptr(size), 0)
		if r == crBufferSmall {
			continue // The list changed between the two calls.
		}
		if r != crSuccess {
			return nil, fmt.Errorf("CM_Get_Device_Interface_ListW: CONFIGRET=0x%X", r)
		}
		var paths []string
		start := 0
		for i, c := range buffer {
			if c != 0 {
				continue
			}
			if i > start {
				if path := syscall.UTF16ToString(buffer[start:i]); strings.Contains(strings.ToLower(path), target) {
					paths = append(paths, path)
				}
			}
			start = i + 1
		}
		return paths, nil
	}
}

func productName(id string) (string, error) {
	name, err := syscall.UTF16PtrFromString(id)
	if err != nil {
		return "", err
	}
	var node uint32
	if r, _, _ := procLocateDevNode.Call(uintptr(unsafe.Pointer(&node)), uintptr(unsafe.Pointer(name)), 0); r != crSuccess {
		return "", fmt.Errorf("CM_Locate_DevNodeW: CONFIGRET=0x%X", r)
	}
	var propertyType, size uint32
	buffer := make([]uint16, 256)
	size = uint32(len(buffer) * 2)
	if r, _, _ := procGetDevNodeProperty.Call(uintptr(node), uintptr(unsafe.Pointer(&busReportedDeviceDesc)), uintptr(unsafe.Pointer(&propertyType)), uintptr(unsafe.Pointer(&buffer[0])), uintptr(unsafe.Pointer(&size)), 0); r != crSuccess {
		return "", fmt.Errorf("CM_Get_DevNode_PropertyW: CONFIGRET=0x%X", r)
	}
	return syscall.UTF16ToString(buffer), nil
}
