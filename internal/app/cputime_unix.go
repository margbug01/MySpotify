//go:build !windows

package app

import "syscall"

// cpuTime is the process's CPU time so far, in ms, all its threads.
func cpuTime() int64 {
	var ru syscall.Rusage
	syscall.Getrusage(syscall.RUSAGE_SELF, &ru)
	return (ru.Utime.Nano() + ru.Stime.Nano()) / 1e6
}
