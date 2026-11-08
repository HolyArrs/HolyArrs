package health

import (
	"strings"
	"testing"
)

// Podman, Kubernetes and containerd leave no /.dockerenv, but the image sets
// container=oci. About (and so the update steps on Status) must still say
// "in a container", not "replace the program".
func TestAboutSeesAnyContainer(t *testing.T) {
	t.Setenv("container", "oci")
	srv, _, _ := setup(t)
	b := string(do(t, "GET", srv.URL+"/api/v1/app/about", "", 200))
	if !strings.Contains(b, `"docker":true`) {
		t.Fatalf("about does not see the container: %s", b)
	}
}
