package main

import (
	"testing"
)

// Podman, Kubernetes and containerd leave no /.dockerenv, but the image sets
// container=oci. The arr status must still say it runs in a container.
func TestOldAppStatusSeesAnyContainer(t *testing.T) {
	t.Setenv("container", "oci")
	f := newOldAppFix(t)
	f.turnOn()
	got := f.arrOK(f.sonarrKey, "GET", "/sonarr/api/v3/system/status", "", 200).(map[string]any)
	if got["isDocker"] != true {
		t.Fatalf("isDocker = %v", got["isDocker"])
	}
}
