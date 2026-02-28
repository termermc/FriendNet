package main

import (
	"syscall"
	"unsafe"
)

var user32 = syscall.NewLazyDLL("user32.dll")

func InfoBox(title string, content string) {
	messageBoxW := user32.NewProc("MessageBoxW")

	titlePtr, _ := syscall.UTF16PtrFromString(title)
	contentPtr, _ := syscall.UTF16PtrFromString(content)

	// Call MessageBoxW.
	// Parameters: hwnd (0 for no parent), lpText, lpCaption, uType
	// uType: 0x00 = OK button, 0x00 = no icon (use 0x40 for info icon)
	_, _, _ = messageBoxW.Call(
		uintptr(0),                          // hwnd - no parent window
		uintptr(unsafe.Pointer(contentPtr)), // lpText
		uintptr(unsafe.Pointer(titlePtr)),   // lpCaption
		uintptr(0x40),                       // uType - MB_ICONINFORMATION
	)
}
