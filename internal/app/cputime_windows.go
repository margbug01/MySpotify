package app

import "syscall"

// cpuTime is the process's CPU time so far, in ms, all its threads.
func cpuTime() int64 {
	var created, exited, kernel, user syscall.Filetime
	h, _ := syscall.GetCurrentProcess()
	syscall.GetProcessTimes(h, &created, &exited, &kernel, &user)
	// Filetimes here are durations in 100 ns ticks; Nanoseconds would
	// take them as dates since 1601.
	ticks := func(f syscall.Filetime) int64 { return int64(f.HighDateTime)<<32 | int64(f.LowDateTime) }
	return (ticks(kernel) + ticks(user)) / 1e4
}
