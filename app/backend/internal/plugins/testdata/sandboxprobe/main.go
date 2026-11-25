// Command sandboxprobe tries one action and exits 0 only if it worked.
// Usage: sandboxprobe read|write <path> | signalparent | fork <pidfile> | escape <pidfile> | forkmany <n> | sleep |
// httpget <url> | tls <addr> <name> | tcp <addr> | udp | socket <family> <type> | iouring | unix <path> | symlink <target> <link>
package main

import (
	"crypto/tls"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

func main() {
	var err error
	switch os.Args[1] {
	case "read":
		_, err = os.ReadFile(os.Args[2])
	case "write":
		err = os.WriteFile(os.Args[2], []byte("x"), 0o600)
	case "signalparent":
		// SIGURG is harmless to a Go parent, so a missed lockdown does no damage.
		err = syscall.Kill(os.Getppid(), syscall.SIGURG)
	case "fork":
		child := exec.Command(os.Args[0], "sleep")
		// Inherit stdio: the sandbox may not open /dev/null for writing.
		child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
		if err = child.Start(); err == nil {
			err = os.WriteFile(os.Args[2], []byte(strconv.Itoa(child.Process.Pid)), 0o600)
		}
	case "escape":
		// A grandchild in its own session, out of the process group.
		child := exec.Command(os.Args[0], "sleep")
		child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
		child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
		if err = child.Start(); err == nil {
			err = os.WriteFile(os.Args[2], []byte(strconv.Itoa(child.Process.Pid)), 0o600)
		}
	case "forkmany":
		// Exits 0 only when a start fails before n children run.
		n, _ := strconv.Atoi(os.Args[2])
		err = fmt.Errorf("started all %d children", n)
		for i := 0; i < n; i++ {
			child := exec.Command(os.Args[0], "sleep")
			child.Stdin, child.Stdout, child.Stderr = os.Stdin, os.Stdout, os.Stderr
			if child.Start() != nil {
				err = nil
				break
			}
		}
	case "sleep":
		time.Sleep(time.Hour)
	case "httpget":
		// The default client honours HTTPS_PROXY / HTTP_PROXY / NO_PROXY.
		c := &http.Client{Timeout: 15 * time.Second, Transport: &http.Transport{Proxy: http.ProxyFromEnvironment,
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}}
		var resp *http.Response
		if resp, err = c.Get(os.Args[2]); err == nil {
			b, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode != 200 {
				err = fmt.Errorf("status %d: %s", resp.StatusCode, b)
			} else {
				fmt.Printf("body %s\n", b)
			}
		}
	case "tls":
		// Straight to an address, ignoring the proxy settings, with a server name.
		d := &net.Dialer{Timeout: 15 * time.Second}
		var c *tls.Conn
		if c, err = tls.DialWithDialer(d, "tcp", os.Args[2], &tls.Config{ServerName: os.Args[3], InsecureSkipVerify: true}); err == nil {
			c.Close()
		}
	case "tcp":
		// Connect and read one byte back: refused connections end before any.
		var c net.Conn
		if c, err = net.DialTimeout("tcp", os.Args[2], 15*time.Second); err == nil {
			c.SetDeadline(time.Now().Add(20 * time.Second))
			io.WriteString(c, "GET / HTTP/1.0\r\nHost: x\r\n\r\n")
			b := make([]byte, 12)
			if _, err = io.ReadFull(c, b); err == nil && !strings.HasSuffix(string(b), " 200") {
				err = fmt.Errorf("answer %q", b)
			}
			c.Close()
		}
	case "udp":
		var pc net.PacketConn
		if pc, err = net.ListenPacket("udp4", "0.0.0.0:0"); err == nil {
			pc.Close()
		}
	case "socket":
		fam, _ := strconv.Atoi(os.Args[2])
		typ, _ := strconv.Atoi(os.Args[3])
		var fd int
		if fd, err = syscall.Socket(fam, typ, 0); err == nil {
			syscall.Close(fd)
		}
	case "iouring":
		params := make([]byte, 120)
		if _, _, e := syscall.Syscall(425, 4, uintptr(unsafePtr(params)), 0); e != 0 {
			err = e
		}
	case "unix":
		var c net.Conn
		if c, err = net.Dial("unix", os.Args[2]); err == nil {
			c.Close()
		}
	case "symlink":
		err = os.Symlink(os.Args[2], os.Args[3])
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
