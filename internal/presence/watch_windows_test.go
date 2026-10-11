//go:build windows

package presence

import (
	"context"
	"runtime"
	"syscall"
	"testing"
	"time"
	"unsafe"
)

// The hidden window gets the same messages that Windows posts for the power and session changes.
func TestWatchReportsPowerAndSessionMessages(t *testing.T) {
	got := make(chan bool, 8)
	c := New(func(away bool) { got <- away })
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		Watch(ctx, c, func() int { return 1 << 30 })
		close(done)
	}()
	class, _ := syscall.UTF16PtrFromString("TokenMonitorTurzxPresence")
	title, _ := syscall.UTF16PtrFromString(WindowTitle())
	var hwnd uintptr
	for i := 0; i < 100 && hwnd == 0; i++ {
		hwnd, _, _ = user32.NewProc("FindWindowW").Call(uintptr(unsafe.Pointer(class)), uintptr(unsafe.Pointer(title)))
		time.Sleep(20 * time.Millisecond)
	}
	if hwnd == 0 {
		t.Fatal("the hidden window was not created")
	}
	send := func(message, wParam, lParam uintptr, want bool) {
		t.Helper()
		procPostMessage.Call(hwnd, message, wParam, lParam)
		select {
		case v := <-got:
			if v != want {
				t.Fatalf("away = %v, want %v", v, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("no change, want away = %v", want)
		}
	}
	display := func(state byte) {
		setting := &powerBroadcastSetting{powerSetting: consoleDisplayState, dataLength: 1, data: [1]byte{state}}
		// Windows refuses to post a message that carries an address, so it is sent and handled at once.
		user32.NewProc("SendMessageW").Call(hwnd, wmPowerBroadcast, pbtPowerSetting, uintptr(unsafe.Pointer(setting)))
		runtime.KeepAlive(setting)
	}
	send(wmSessionChange, wtsLock, 0, true)
	send(wmSessionChange, wtsUnlock, 0, false)
	send(wmSessionChange, wtsRemoteConnect, 0, true)
	send(wmSessionChange, wtsRemoteDisc, 0, false)
	send(wmPowerBroadcast, pbtSuspend, 0, true)
	send(wmPowerBroadcast, pbtResumeAuto, 0, false)
	display(0)
	select {
	case v := <-got:
		if !v {
			t.Fatal("monitor off did not make the user away")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("monitor off was not reported")
	}
	display(1)
	select {
	case v := <-got:
		if v {
			t.Fatal("monitor on kept the user away")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("monitor on was not reported")
	}
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("the watcher did not stop")
	}
}
