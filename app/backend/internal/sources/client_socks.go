package sources

import (
	"context"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"strconv"
	"time"
)

// socks5Connect asks a SOCKS5 proxy to connect conn to addr, following RFC 1928 and RFC 1929 for the name and password.
// An IP address is sent as an address and a name is sent as it is, so the proxy looks it up.
func socks5Connect(ctx context.Context, conn net.Conn, addr, user, pass string) error {
	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	} else {
		// Without a caller deadline the handshake still must not hang.
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	}
	defer conn.SetDeadline(time.Time{})
	host, portS, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	port, err := strconv.Atoi(portS)
	if err != nil || port < 1 || port > 65535 || host == "" || len(host) > 255 {
		return errors.New("socks5: bad address")
	}
	method := byte(0x00)
	if user != "" {
		method = 0x02
	}
	if _, err := conn.Write([]byte{5, 1, method}); err != nil {
		return err
	}
	var b [2]byte
	if _, err := io.ReadFull(conn, b[:]); err != nil {
		return err
	}
	if b[0] != 5 || b[1] != method {
		return errors.New("socks5: the proxy refused the sign-in method")
	}
	if method == 0x02 {
		if len(user) > 255 || len(pass) > 255 {
			return errors.New("socks5: name or password too long")
		}
		msg := append([]byte{1, byte(len(user))}, user...)
		msg = append(append(msg, byte(len(pass))), pass...)
		if _, err := conn.Write(msg); err != nil {
			return err
		}
		if _, err := io.ReadFull(conn, b[:]); err != nil {
			return err
		}
		if b[0] != 1 || b[1] != 0 {
			return errors.New("socks5: the proxy turned down the name or password")
		}
	}
	req := []byte{5, 1, 0}
	if ip := net.ParseIP(host); ip != nil && ip.To4() != nil {
		req = append(append(req, 1), ip.To4()...)
	} else if ip != nil {
		req = append(append(req, 4), ip.To16()...)
	} else {
		req = append(append(req, 3, byte(len(host))), host...)
	}
	req = binary.BigEndian.AppendUint16(req, uint16(port))
	if _, err := conn.Write(req); err != nil {
		return err
	}
	var head [4]byte
	if _, err := io.ReadFull(conn, head[:]); err != nil {
		return err
	}
	if head[0] != 5 {
		return errors.New("socks5: bad answer")
	}
	if head[1] != 0 {
		return errors.New("socks5: the proxy could not connect (code " + strconv.Itoa(int(head[1])) + ")")
	}
	skip := 0
	switch head[3] {
	case 1:
		skip = 4
	case 4:
		skip = 16
	case 3:
		var n [1]byte
		if _, err := io.ReadFull(conn, n[:]); err != nil {
			return err
		}
		skip = int(n[0])
	default:
		return errors.New("socks5: bad answer")
	}
	_, err = io.ReadFull(conn, make([]byte, skip+2))
	return err
}
