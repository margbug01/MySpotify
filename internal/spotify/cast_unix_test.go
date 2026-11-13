//go:build darwin || linux

package spotify

import (
	"errors"
	"syscall"
	"testing"
)

// Setting mDNS socket options on a UDP socket enables SO_REUSEADDR and SO_REUSEPORT on Darwin and Linux.
func TestCastMDNSSocketOptions(t *testing.T) {
	fd, err := syscall.Socket(syscall.AF_INET, syscall.SOCK_DGRAM, 0)
	if err != nil {
		if errors.Is(err, syscall.EPERM) || errors.Is(err, syscall.EACCES) {
			t.Skipf("sandbox forbids raw UDP socket creation: %v", err)
		}
		t.Fatalf("syscall.Socket failed: %v", err)
	}
	defer syscall.Close(fd)

	if err := setMDNSSocketOptions(uintptr(fd)); err != nil {
		t.Fatalf("setMDNSSocketOptions failed: %v", err)
	}

	reuseAddr, err := syscall.GetsockoptInt(fd, syscall.SOL_SOCKET, syscall.SO_REUSEADDR)
	if err != nil {
		t.Fatalf("GetsockoptInt(SO_REUSEADDR) failed: %v", err)
	}
	if reuseAddr == 0 {
		t.Fatal("expected SO_REUSEADDR to be enabled")
	}

	reusePort, err := syscall.GetsockoptInt(fd, syscall.SOL_SOCKET, soReusePort)
	if err != nil {
		t.Fatalf("GetsockoptInt(SO_REUSEPORT) failed: %v", err)
	}
	if reusePort == 0 {
		t.Fatal("expected SO_REUSEPORT to be enabled")
	}
}
