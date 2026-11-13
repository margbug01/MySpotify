//go:build !windows

package spotify

import "syscall"

func joinMDNSMulticast(fd uintptr) error {
	mreq := syscall.IPMreq{
		Multiaddr: [4]byte{224, 0, 0, 251},
		Interface: [4]byte{0, 0, 0, 0},
	}
	return syscall.SetsockoptIPMreq(int(fd), syscall.IPPROTO_IP, syscall.IP_ADD_MEMBERSHIP, &mreq)
}
