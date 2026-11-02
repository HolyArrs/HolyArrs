//go:build !unix

package upstream

import "syscall"

func reuseAddr(syscall.RawConn) error { return nil }
