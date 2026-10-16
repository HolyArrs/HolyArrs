package main

import (
	"os"
	"testing"
)

// TestMain keeps the update check off the real network: the release feed
// points at a loopback address the public-only policy refuses before dialing.
func TestMain(m *testing.M) {
	_ = os.Setenv("HOLYARRS_RELEASE_FEED", "http://127.0.0.1:1/no-network-in-tests")
	os.Exit(m.Run())
}
