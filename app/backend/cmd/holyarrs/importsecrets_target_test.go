package main

import (
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/migrate/secretsfile"
)

// Plain http carries the owner key and the whole file, so it goes only to
// this machine unless -allow-http-lan says otherwise; https goes anywhere
// but a cloud metadata address.
func TestAppTargetHTTPRule(t *testing.T) {
	for _, c := range []struct {
		addr     string
		lan      bool
		ok, warn bool
	}{
		{"http://127.0.0.1:18730", false, true, false},
		{"http://localhost:18730", false, true, false},
		{"http://[::1]:18730", false, true, false},
		{"https://holyarrs.example.com", false, true, false},
		{"https://192.168.1.10:18730", false, true, false},
		{"http://192.168.1.10:18730", false, false, false},
		{"http://holyarrs:18730", false, false, false},
		{"http://nas.lan:18730", false, false, false},
		{"http://192.168.1.10:18730", true, true, true},
		{"http://holyarrs:18730", true, true, true},
		{"http://holyarrs.example.com", true, false, false},
		{"http://169.254.10.1:18730", true, false, false},
		{"http://169.254.169.254", true, false, false},
		{"https://169.254.169.254", false, false, false},
		{"https://[fd00:ec2::254]", false, false, false},
		{"https://100.100.100.200", false, false, false},
	} {
		f := &secretsfile.File{}
		f.HolyArrs.Address, f.HolyArrs.OwnerAPIKey = c.addr, "your-owner-key-6"
		var warned []string
		_, _, err := appTarget(f, c.lan, func(format string, a ...any) { warned = append(warned, format) })
		if (err == nil) != c.ok {
			t.Errorf("%s lan=%v: err %v", c.addr, c.lan, err)
		}
		if (len(warned) > 0) != c.warn {
			t.Errorf("%s lan=%v: warned %v", c.addr, c.lan, warned)
		}
		if err != nil && strings.Contains(err.Error(), "your-owner") {
			t.Error("key in error")
		}
	}
}
