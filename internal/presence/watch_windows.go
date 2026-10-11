//go:build windows

package presence

import (
	"context"
	"runtime"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	user32                 = windows.NewLazySystemDLL("user32.dll")
	kernel32               = windows.NewLazySystemDLL("kernel32.dll")
	wtsapi32               = windows.NewLazySystemDLL("wtsapi32.dll")
	procRegisterClass      = user32.NewProc("RegisterClassExW")
	procCreateWindow       = user32.NewProc("CreateWindowExW")
	procDefWindowProc      = user32.NewProc("DefWindowProcW")
	procGetMessage         = user32.NewProc("GetMessageW")
	procDispatchMessage    = user32.NewProc("DispatchMessageW")
	procPostMessage        = user32.NewProc("PostMessageW")
	procPostQuitMessage    = user32.NewProc("PostQuitMessage")
	procRegisterPowerSet   = user32.NewProc("RegisterPowerSettingNotification")
	procGetLastInputInfo   = user32.NewProc("GetLastInputInfo")
	procGetTickCount       = kernel32.NewProc("GetTickCount")
	procRegisterSessionNot = wtsapi32.NewProc("WTSRegisterSessionNotification")
)

const (
	wmDestroy         = 0x0002
	wmClose           = 0x0010
	wmPowerBroadcast  = 0x0218
	wmSessionChange   = 0x02B1
	pbtSuspend        = 0x0004
	pbtResumeSuspend  = 0x0007
	pbtResumeAuto     = 0x0012
	pbtPowerSetting   = 0x8013
	notifyThisSession = 0
	deviceNotifyWnd   = 0
	wtsConsoleConnect = 1
	wtsConsoleDisc    = 2
	wtsRemoteConnect  = 3
	wtsRemoteDisc     = 4
	wtsLogon          = 5
	wtsLogoff         = 6
	wtsLock           = 7
	wtsUnlock         = 8
)

// GUID_CONSOLE_DISPLAY_STATE: 0 off, 1 on, 2 dimmed.
var consoleDisplayState = windows.GUID{Data1: 0x6fe69556, Data2: 0x704a, Data3: 0x47a0, Data4: [8]byte{0x8f, 0x24, 0xc2, 0x8d, 0x93, 0x6f, 0xda, 0x47}}

type wndClass struct {
	size       uint32
	style      uint32
	proc       uintptr
	classExtra int32
	wndExtra   int32
	instance   windows.Handle
	icon       windows.Handle
	cursor     windows.Handle
	background windows.Handle
	menu       *uint16
	className  *uint16
	iconSmall  windows.Handle
}

type msg struct {
	hwnd    windows.HWND
	message uint32
	wParam  uintptr
	lParam  uintptr
	time    uint32
	pt      struct{ x, y int32 }
}

type powerBroadcastSetting struct {
	powerSetting windows.GUID
	dataLength   uint32
	data         [1]byte
}

// Watch reports the conditions that Windows signals, and Idle when there has been no input for as long
// as idleAfter returns (seconds). It blocks until ctx ends.
func Watch(ctx context.Context, c *Conditions, idleAfter func() int) {
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				c.Set(Idle, idleSeconds() >= idleAfter())
			}
		}
	}()
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	proc := windows.NewCallback(func(hwnd windows.HWND, message uint32, wParam, lParam uintptr) uintptr {
		switch message {
		case wmPowerBroadcast:
			switch wParam {
			case pbtSuspend:
				c.Set(Suspended, true)
			case pbtResumeSuspend, pbtResumeAuto:
				c.Set(Suspended, false)
			case pbtPowerSetting:
				setting := (*powerBroadcastSetting)(*(*unsafe.Pointer)(unsafe.Pointer(&lParam)))
				if setting.powerSetting == consoleDisplayState && setting.dataLength > 0 {
					c.Set(MonitorOff, setting.data[0] == 0)
				}
			}
			return 1
		case wmSessionChange:
			switch wParam {
			case wtsConsoleDisc, wtsRemoteConnect, wtsLogoff, wtsLock:
				c.Set(SessionInactive, true)
			case wtsConsoleConnect, wtsRemoteDisc, wtsLogon, wtsUnlock:
				c.Set(SessionInactive, false)
			}
			return 0
		case wmDestroy:
			procPostQuitMessage.Call(0)
			return 0
		}
		r, _, _ := procDefWindowProc.Call(uintptr(hwnd), uintptr(message), wParam, lParam)
		return r
	})
	name, _ := syscall.UTF16PtrFromString("TokenMonitorTurzxPresence")
	class := wndClass{proc: proc, className: name}
	class.size = uint32(unsafe.Sizeof(class))
	if r, _, _ := procRegisterClass.Call(uintptr(unsafe.Pointer(&class))); r == 0 {
		return
	}
	// A hidden window that is never shown still gets the power and session notifications.
	hwnd, _, _ := procCreateWindow.Call(0, uintptr(unsafe.Pointer(name)), uintptr(unsafe.Pointer(name)), 0, 0, 0, 0, 0, 0, 0, 0, 0)
	if hwnd == 0 {
		return
	}
	procRegisterPowerSet.Call(hwnd, uintptr(unsafe.Pointer(&consoleDisplayState)), deviceNotifyWnd)
	procRegisterSessionNot.Call(hwnd, notifyThisSession)
	go func() {
		<-ctx.Done()
		procPostMessage.Call(hwnd, wmClose, 0, 0)
	}()
	var m msg
	for {
		if r, _, _ := procGetMessage.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0); int32(r) <= 0 {
			return
		}
		procDispatchMessage.Call(uintptr(unsafe.Pointer(&m)))
	}
}

// idleSeconds is the time since the last keyboard or mouse input of the whole session.
func idleSeconds() int {
	info := struct{ size, time uint32 }{size: 8}
	if r, _, _ := procGetLastInputInfo.Call(uintptr(unsafe.Pointer(&info))); r == 0 {
		return 0
	}
	now, _, _ := procGetTickCount.Call()
	return int((uint32(now) - info.time) / 1000)
}
