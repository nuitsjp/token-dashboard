//go:build windows

package localusage

import (
	"context"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

// hideWindow keeps the console tokscale from opening a window from the GUI app.
func hideWindow(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{CreationFlags: windows.CREATE_NO_WINDOW}
}

// dirWatch is larger than 64 KiB so that it lives on the heap and does not move
// while Windows writes to it.
type dirWatch struct {
	overlapped windows.Overlapped
	buffer     [64 << 10]byte
}

// watch calls changed whenever a file or directory under path is created, written, removed or renamed,
// until ctx is canceled.
func watch(ctx context.Context, path string, changed func()) error {
	name, err := windows.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	handle, err := windows.CreateFile(name, windows.FILE_LIST_DIRECTORY,
		windows.FILE_SHARE_READ|windows.FILE_SHARE_WRITE|windows.FILE_SHARE_DELETE, nil,
		windows.OPEN_EXISTING, windows.FILE_FLAG_BACKUP_SEMANTICS|windows.FILE_FLAG_OVERLAPPED, 0)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	event, err := windows.CreateEvent(nil, 1, 0, nil)
	if err != nil {
		return err
	}
	defer windows.CloseHandle(event)
	stop := context.AfterFunc(ctx, func() { windows.CancelIoEx(handle, nil) })
	defer stop()

	const filter = windows.FILE_NOTIFY_CHANGE_FILE_NAME | windows.FILE_NOTIFY_CHANGE_DIR_NAME |
		windows.FILE_NOTIFY_CHANGE_SIZE | windows.FILE_NOTIFY_CHANGE_LAST_WRITE
	w := new(dirWatch)
	for {
		w.overlapped = windows.Overlapped{HEvent: event}
		if err := windows.ReadDirectoryChanges(handle, &w.buffer[0], uint32(len(w.buffer)), true, filter, nil, &w.overlapped, 0); err != nil {
			return err
		}
		// The cancel may have run before this read started.
		if ctx.Err() != nil {
			windows.CancelIoEx(handle, nil)
		}
		var n uint32
		if err := windows.GetOverlappedResult(handle, &w.overlapped, &n, true); err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		// An overflow (n == 0) also means something changed.
		changed()
	}
}
