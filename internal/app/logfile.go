package app

import (
	"os"
	"runtime/debug"
	"sync"
)

// maxLog is how large the log grows before it starts over, the last one
// kept beside it.
const maxLog = 1 << 20

// logFile is the log as a file, vinyl.log in the app's data: the windowed
// app has no console for it. Past maxLog it moves to vinyl.log.1, over
// the one before, and starts again.
type logFile struct {
	mu   sync.Mutex
	path string
	f    *os.File
	n    int64
}

func (l *logFile) Write(b []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil || l.n >= maxLog {
		if err := l.open(); err != nil {
			return 0, err
		}
	}
	n, err := l.f.Write(b)
	l.n += int64(n)
	return n, err
}

// open opens the log to append to, starting over if it is full.
func (l *logFile) open() error {
	if l.f != nil {
		// Windows moves no file open: the crash output's copy included.
		debug.SetCrashOutput(nil, debug.CrashOptions{})
		l.f.Close()
		l.f = nil
	}
	if fi, err := os.Stat(l.path); err == nil && fi.Size() >= maxLog {
		os.Rename(l.path, l.path+".1")
	}
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	fi, err := f.Stat()
	if err != nil {
		f.Close()
		return err
	}
	l.f, l.n = f, fi.Size()
	// A crash's trace goes there too, which would go to the console only.
	debug.SetCrashOutput(f, debug.CrashOptions{})
	return nil
}
