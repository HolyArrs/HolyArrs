package plugins

import "testing"

func TestEntryPlatform(t *testing.T) {
	any := Entry{ID: "tailscale", Version: "1.0.0"}
	if !any.forHost() || any.dir() != "plugins/tailscale/1.0.0/" {
		t.Fatalf("no platform: forHost %v dir %q", any.forHost(), any.dir())
	}
	own := Entry{ID: "tailscale", Version: "1.0.0", Platform: HostPlatform()}
	if !own.forHost() || own.dir() != "plugins/tailscale/1.0.0/"+HostPlatform()+"/" {
		t.Fatalf("own platform: forHost %v dir %q", own.forHost(), own.dir())
	}
	other := Entry{ID: "tailscale", Version: "1.0.0", Platform: "linux-riscv64"}
	if other.forHost() {
		t.Fatal("another platform's build must not be picked")
	}
	for _, p := range []string{"linux-amd64", "linux-arm64", "linux-armv7"} {
		if !platformRe.MatchString(p) {
			t.Fatalf("%s should be allowed", p)
		}
	}
	for _, p := range []string{"windows-amd64", "linux-amd64/../x", "LINUX-AMD64"} {
		if platformRe.MatchString(p) {
			t.Fatalf("%s should be refused", p)
		}
	}
}
