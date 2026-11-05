package api

import (
	"strings"
	"testing"
)

// Podman, Kubernetes and containerd leave no /.dockerenv, but the image sets
// container=oci. About (and so the update steps on Status) and the old-app
// status must still say "in a container", not "replace the program".
func TestAboutSeesAnyContainer(t *testing.T) {
	t.Setenv("container", "oci")
	srv, _, _ := setup(t)
	b := string(do(t, "GET", srv.URL+"/api/v1/app/about", "", 200))
	if !strings.Contains(b, `"docker":true`) {
		t.Fatalf("about does not see the container: %s", b)
	}
}

func TestOldAppStatusSeesAnyContainer(t *testing.T) {
	t.Setenv("container", "oci")
	f := newOldAppFix(t)
	f.turnOn()
	got := f.arrOK(f.sonarrKey, "GET", "/sonarr/api/v3/system/status", "", 200).(map[string]any)
	if got["isDocker"] != true {
		t.Fatalf("isDocker = %v", got["isDocker"])
	}
}
