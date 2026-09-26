//go:build windows && !server

package main

import (
	"syscall"
	"unsafe"
)

func showStartupFailure() {
	title, _ := syscall.UTF16PtrFromString("Wailsアプリを起動できません")
	message, _ := syscall.UTF16PtrFromString("初期化に失敗しました。設定・保存データ・アクセス権を確認してください。保存データを空の内容で上書きしていません。")
	syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW").Call(0, uintptr(unsafe.Pointer(message)), uintptr(unsafe.Pointer(title)), 0x10)
}
