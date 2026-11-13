//go:build linux

package spotify

import "syscall"

const soReusePort = 0x0f

func setMDNSSocketOptions(fd uintptr) error {
	if err := syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, soReusePort, 1); err != nil {
		return err
	}
	return syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1)
}
