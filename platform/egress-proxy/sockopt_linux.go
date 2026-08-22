package main

import (
	"syscall"
	"unsafe"
)

// getsockoptBytes reads a raw socket option, for the one case the typed helpers
// in syscall do not cover: SO_ORIGINAL_DST on IPv6 returns a sockaddr_in6 and
// no helper returns that shape.
func getsockoptBytes(fd, level, opt, size int) ([]byte, error) {
	buf := make([]byte, size)
	l := uint32(size)
	_, _, errno := syscall.Syscall6(syscall.SYS_GETSOCKOPT,
		uintptr(fd), uintptr(level), uintptr(opt),
		uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&l)), 0)
	if errno != 0 {
		return nil, errno
	}
	return buf[:l], nil
}
