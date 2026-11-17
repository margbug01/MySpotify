package app

import (
	"syscall"
	"unsafe"
)

var dwmSetWindowAttribute = syscall.NewLazyDLL("dwmapi.dll").NewProc("DwmSetWindowAttribute")

// undecorate takes from the desktop lyrics' window the frame Windows 11
// draws around every window, a transparent one too: a grey line and a
// shadow showed the window's bounds around the lyrics (#41). Square
// corners leave it no shadow, the border color none no line.
func undecorate(handle uintptr) {
	if dwmSetWindowAttribute.Find() != nil {
		return
	}
	const (
		cornerPreference = 33 // DWMWA_WINDOW_CORNER_PREFERENCE
		borderColor      = 34 // DWMWA_BORDER_COLOR
	)
	square, none := uint32(1), uint32(0xFFFFFFFE) // DWMWCP_DONOTROUND, DWMWA_COLOR_NONE
	dwmSetWindowAttribute.Call(handle, cornerPreference, uintptr(unsafe.Pointer(&square)), 4)
	dwmSetWindowAttribute.Call(handle, borderColor, uintptr(unsafe.Pointer(&none)), 4)
}
