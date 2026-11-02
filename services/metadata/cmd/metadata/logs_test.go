package main

import (
	"bytes"
	"log"
	"testing"
)

func TestLogsKeepNoAddresses(t *testing.T) {
	var b bytes.Buffer
	l := log.New(addressFree{&b}, "", 0)
	l.Printf("http: TLS handshake error from 203.0.113.7:52114: EOF")
	l.Printf("http2: error reading preface from client [2001:db8::1]:443 and 2001:db8:85a3::8a2e:370:7334")
	l.Printf("2026/10/26 22:07:30 yt-dlp 2026.08.19, sqlite 1.60.1, 10.0.0.1 asked")
	got := b.String()
	for _, leak := range []string{"203.0.113.7", "2001:db8", "10.0.0.1"} {
		if bytes.Contains([]byte(got), []byte(leak)) {
			t.Fatalf("address kept: %q", got)
		}
	}
	for _, keep := range []string{"22:07:30", "2026.08.19", "1.60.1", "[address]: EOF"} {
		if !bytes.Contains([]byte(got), []byte(keep)) {
			t.Fatalf("%q lost: %q", keep, got)
		}
	}
}
