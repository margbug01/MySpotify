//go:build windows

package main

import (
	"log"
	"unsafe"

	"golang.org/x/sys/windows"
)

func setAppIdentity() {
	id, err := windows.UTF16PtrFromString("dev.margbug.MySpotify")
	if err != nil {
		log.Printf("app identity: %v", err)
		return
	}
	proc := windows.NewLazySystemDLL("shell32.dll").NewProc("SetCurrentProcessExplicitAppUserModelID")
	if hr, _, _ := proc.Call(uintptr(unsafe.Pointer(id))); hr != 0 {
		log.Printf("app identity: SetCurrentProcessExplicitAppUserModelID failed: %#x", hr)
	}
}
