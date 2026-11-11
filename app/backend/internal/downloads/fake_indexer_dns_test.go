package downloads

import "net"

// The test Releases link to http://fake-indexer/, which no resolver knows. A
// qBittorrent grab reads that link first, and on Windows the failed lookup of a
// one-word name takes about 3 seconds, on nearly every test here. Pointing the
// name at a loopback address the grab dialer refuses gives the same failed read
// at once. fakeDNS swaps this out and puts it back.
func init() {
	grabDial = func(addr string) string {
		if host, _, err := net.SplitHostPort(addr); err == nil && host == "fake-indexer" {
			return "127.0.0.1:9"
		}
		return addr
	}
}
