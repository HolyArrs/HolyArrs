package main

import (
	"io"
	"net"
	"regexp"
	"strings"
)

// addressFree writes log lines with every IP address (IPv4 or IPv6, with or
// without a port) replaced by [address], so the server's logs never keep
// an install's address (net/http's own errors name the remote address).
type addressFree struct{ w io.Writer }

func (a addressFree) Write(p []byte) (int, error) {
	if _, err := a.w.Write(redactAddresses(p)); err != nil {
		return 0, err
	}
	return len(p), nil
}

var addrToken = regexp.MustCompile(`\[?[0-9A-Fa-f:.]{7,}\]?(?::\d+)?`)

func redactAddresses(p []byte) []byte {
	return addrToken.ReplaceAllFunc(p, func(tok []byte) []byte {
		// A colon or full stop after the address belongs to the sentence.
		word := strings.TrimRight(string(tok), ":.")
		host := word
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		if net.ParseIP(strings.Trim(host, "[]")) != nil {
			return []byte("[address]" + string(tok[len(word):]))
		}
		return tok
	})
}
