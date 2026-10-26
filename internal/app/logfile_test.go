package app

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime/debug"
	"testing"
)

// Past a megabyte the log moves aside, over the one before, and starts over.
func TestLogStartsOver(t *testing.T) {
	path := filepath.Join(t.TempDir(), "vinyl.log")
	os.WriteFile(path+".1", []byte("older\n"), 0o600)
	l := &logFile{path: path}
	defer func() {
		l.f.Close()
		// It holds the file open for a crash's trace, which the cleanup
		// could not then remove on Windows.
		debug.SetCrashOutput(nil, debug.CrashOptions{})
	}()
	line := append(bytes.Repeat([]byte("x"), 999), '\n')
	for range maxLog/len(line) + 10 {
		if _, err := l.Write(line); err != nil {
			t.Fatal(err)
		}
	}
	old, err := os.Stat(path + ".1")
	if err != nil || old.Size() < maxLog {
		t.Fatalf("moved aside: %v, %v", old, err)
	}
	cur, err := os.Stat(path)
	if err != nil || cur.Size() == 0 || cur.Size() > 10*int64(len(line)) {
		t.Fatalf("started over: %v, %v", cur, err)
	}
}
