//go:build unix

package upstream

import "syscall"

func reuseAddr(c syscall.RawConn) error {
	var serr error
	err := c.Control(func(fd uintptr) { serr = syscall.SetsockoptInt(int(fd), syscall.SOL_SOCKET, syscall.SO_REUSEADDR, 1) })
	if err != nil {
		return err
	}
	return serr
}
