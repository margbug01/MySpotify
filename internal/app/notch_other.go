//go:build !darwin

package app

func notchSize() (w, h float64, ok bool) { return 0, 0, false }

func raiseAboveMenuBar(handle uintptr) {}

func pinToTop(handle uintptr, w, h float64) {}

func windowFrame(handle uintptr) (x, y, w, h float64) { return }

func menuBarHeight() float64 { return 0 }

func watchPointer(fn func()) (stop func(), ok bool) { return func() {}, false }
