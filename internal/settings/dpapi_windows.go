//go:build windows

package settings

import (
	"syscall"
	"unsafe"
)

var (
	crypt32           = syscall.NewLazyDLL("crypt32.dll")
	procProtectData   = crypt32.NewProc("CryptProtectData")
	procUnprotectData = crypt32.NewProc("CryptUnprotectData")
	procLocalFree     = syscall.NewLazyDLL("kernel32.dll").NewProc("LocalFree")
)

type dataBlob struct {
	size uint32
	data *byte
}

func blob(b []byte) *dataBlob {
	if len(b) == 0 {
		return &dataBlob{}
	}
	return &dataBlob{size: uint32(len(b)), data: &b[0]}
}

const cryptProtectUIForbidden = 0x1

// protect encrypts for the current Windows user with DPAPI.
func protect(plain, entropy []byte) ([]byte, error) { return crypt(procProtectData, plain, entropy) }

func unprotect(sealed, entropy []byte) ([]byte, error) {
	return crypt(procUnprotectData, sealed, entropy)
}

// CryptProtectData and CryptUnprotectData share the same argument layout.
func crypt(proc *syscall.LazyProc, in, entropy []byte) ([]byte, error) {
	var out dataBlob
	r, _, err := proc.Call(uintptr(unsafe.Pointer(blob(in))), 0, uintptr(unsafe.Pointer(blob(entropy))), 0, 0,
		cryptProtectUIForbidden, uintptr(unsafe.Pointer(&out)))
	if r == 0 {
		return nil, err
	}
	defer procLocalFree.Call(uintptr(unsafe.Pointer(out.data)))
	return append([]byte(nil), unsafe.Slice(out.data, out.size)...), nil
}
