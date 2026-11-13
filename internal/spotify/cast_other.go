//go:build !darwin && !linux && !windows

package spotify

import "syscall"

func setMDNSSocketOptions(fd uintptr) error {
	return syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
}
