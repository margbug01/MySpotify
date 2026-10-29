//go:build darwin

package player

import (
	"runtime"
	"testing"
)

// WebKit runs on the main thread only: init, which runs there, keeps the
// main goroutine on it, and runTests has it carry out onMain's calls while
// the tests run on others.
func init() { runtime.LockOSThread() }

var mainCalls = make(chan func())

func runTests(m *testing.M) int {
	done := make(chan int)
	go func() { done <- m.Run() }()
	for {
		select {
		case code := <-done:
			return code
		case fn := <-mainCalls:
			fn()
		}
	}
}

// onMain runs fn on the main thread.
func onMain(fn func()) {
	ran := make(chan struct{})
	mainCalls <- func() { defer close(ran); fn() }
	<-ran
}
