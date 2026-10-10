//go:build windows

package desktop

import (
	"os"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/wailsapp/wails/v3/pkg/application"

	"token-monitor-turzx/internal/fault"
)

const (
	bifReturnOnlyFSDirs   = 0x0001
	bifEditBox            = 0x0010
	bifNewDialogStyle     = 0x0040
	bifNoNewFolderButton  = 0x0200
	bifBrowseIncludeFiles = 0x4000
	bffmInitialized       = 1
	bffmSelChanged        = 2
	wmSetText             = 0x000C
	wmCommand             = 0x0111
	enChange              = 0x0300
	folderEditID          = 0x3744
	bffmEnableOK          = 0x0400 + 101
	coinitApartment       = 0x2
	rpcEChangedMode       = 0x80010106
)

var (
	shell32        = syscall.NewLazyDLL("shell32.dll")
	ole32          = syscall.NewLazyDLL("ole32.dll")
	user32         = syscall.NewLazyDLL("user32.dll")
	procBrowse     = shell32.NewProc("SHBrowseForFolderW")
	procPath       = shell32.NewProc("SHGetPathFromIDListEx")
	procFree       = ole32.NewProc("CoTaskMemFree")
	procCoInit     = ole32.NewProc("CoInitializeEx")
	procCoUninit   = ole32.NewProc("CoUninitialize")
	procSend       = user32.NewProc("SendMessageW")
	procGetDlgItem = user32.NewProc("GetDlgItem")
	procGetText    = user32.NewProc("InternalGetWindowText")
	procSetProc    = user32.NewProc("SetWindowLongPtrW")
	procCallProc   = user32.NewProc("CallWindowProcW")
	browseProc     = syscall.NewCallback(onBrowse)
	dialogProc     = syscall.NewCallback(onDialog)
	origDialogProc uintptr
	typedFolder    string
	confirming     bool
	textBuf        [1024]uint16
)

type browseInfo struct {
	hwndOwner      uintptr
	pidlRoot       uintptr
	pszDisplayName *uint16
	lpszTitle      *uint16
	ulFlags        uint32
	lpfn           uintptr
	lParam         uintptr
	iImage         int32
}

func chooseStyleFolder() (string, error) {
	hr, _, _ := procCoInit.Call(0, coinitApartment)
	if hr != rpcEChangedMode {
		defer procCoUninit.Call()
	}
	var owner uintptr
	if window := application.Get().Window.Current(); window != nil {
		owner = uintptr(window.NativeWindow())
	}
	display := make([]uint16, 260)
	title, err := syscall.UTF16PtrFromString("Select the folder that contains the style files.")
	if err != nil {
		return "", err
	}
	info := browseInfo{
		hwndOwner:      owner,
		pszDisplayName: &display[0],
		lpszTitle:      title,
		ulFlags:        bifReturnOnlyFSDirs | bifEditBox | bifNewDialogStyle | bifNoNewFolderButton | bifBrowseIncludeFiles,
		lpfn:           browseProc,
	}
	typedFolder = ""
	confirming = false
	pidl, _, _ := procBrowse.Call(uintptr(unsafe.Pointer(&info)))
	if pidl == 0 {
		return "", nil
	}
	defer procFree.Call(pidl)
	if stat, err := os.Stat(typedFolder); err == nil && stat.IsDir() {
		return typedFolder, nil
	}
	path := pathFromPIDL(pidl)
	stat, err := os.Stat(path)
	if path == "" || err != nil || !stat.IsDir() {
		return "", fault.New("INVALID", "This folder is not a valid style.")
	}
	return path, nil
}

func onBrowse(hwnd, msg, lParam, _ uintptr) uintptr {
	switch msg {
	case bffmInitialized:
		title, err := syscall.UTF16PtrFromString("Select a folder")
		if err == nil {
			procSend.Call(hwnd, wmSetText, 0, uintptr(unsafe.Pointer(title)))
		}
		previous, _, _ := procSetProc.Call(hwnd, ^uintptr(3), dialogProc)
		origDialogProc = previous
	case bffmSelChanged:
		var enable uintptr
		if path := pathFromPIDL(lParam); path != "" {
			if stat, err := os.Stat(path); err == nil && stat.IsDir() {
				enable = 1
			}
		}
		procSend.Call(hwnd, bffmEnableOK, 0, enable)
	}
	return 0
}

func onDialog(hwnd, msg, wParam, lParam uintptr) uintptr {
	if msg == wmCommand {
		id := wParam & 0xFFFF
		notify := wParam >> 16
		if id == folderEditID && notify == enChange && !confirming {
			applyEditPath(hwnd)
		}
		if id == 1 {
			confirming = true
		}
	}
	ret, _, _ := procCallProc.Call(origDialogProc, hwnd, msg, wParam, lParam)
	return ret
}

func applyEditPath(hwnd uintptr) {
	edit, _, _ := procGetDlgItem.Call(hwnd, folderEditID)
	if edit == 0 {
		return
	}
	n, _, _ := procGetText.Call(edit, uintptr(unsafe.Pointer(&textBuf[0])), uintptr(len(textBuf)))
	runtime.KeepAlive(&textBuf)
	if n == 0 {
		typedFolder = ""
		return
	}
	path := syscall.UTF16ToString(textBuf[:])
	stat, err := os.Stat(path)
	if err != nil || !stat.IsDir() {
		typedFolder = ""
		return
	}
	typedFolder = path
	procSend.Call(hwnd, bffmEnableOK, 0, 1)
}

func pathFromPIDL(pidl uintptr) string {
	buf := make([]uint16, 32768)
	hr, _, _ := procPath.Call(pidl, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), 0)
	if hr != 0 {
		return ""
	}
	return syscall.UTF16ToString(buf)
}
