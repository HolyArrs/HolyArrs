package runas

import (
	"strings"
	"syscall"
	"testing"
)

// An unmapped PUID (unprivileged LXC, rootless Docker/Podman) makes
// setgroups/setgid/setuid return EINVAL: say that, not "add capabilities".
func TestSwitchFailedNamesAnUnmappedID(t *testing.T) {
	msg := SwitchFailed(1000, 1000, syscall.EINVAL).Error()
	for _, want := range []string{"does not exist inside this container", "rootless", "PUID/PGID"} {
		if !strings.Contains(msg, want) {
			t.Errorf("EINVAL message lacks %q: %s", want, msg)
		}
	}
	if strings.Contains(msg, "DAC_OVERRIDE") {
		t.Errorf("EINVAL is not a capability problem: %s", msg)
	}
	// EPERM (cap_drop: ALL) keeps today's text.
	perm := SwitchFailed(1000, 1000, syscall.EPERM).Error()
	if !strings.Contains(perm, "CHOWN, DAC_OVERRIDE, SETUID and SETGID") {
		t.Errorf("EPERM text changed: %s", perm)
	}
}

// The owner-fix warning names the real cause instead of always blaming
// NFS or SMB.
func TestOwnerWarningNamesTheCause(t *testing.T) {
	ro := ownerWarning("/config", 1000, 1000, syscall.EROFS, "", "")
	if !strings.Contains(ro, ":ro") || strings.Contains(ro, "chown") {
		t.Errorf("read-only mount: %s", ro)
	}

	// Rootless: the files belong to a host user this container cannot see;
	// the host-side number comes from uid_map.
	unmapped := ownerWarning("/config", 1000, 1000, syscall.EINVAL, "0 100000 65536\n", "")
	if !strings.Contains(unmapped, "chown -R 101000:101000") {
		t.Errorf("unmapped owner with uid_map: %s", unmapped)
	}
	noMap := ownerWarning("/config", 1000, 1000, syscall.EINVAL, "", "")
	if !strings.Contains(noMap, "host user") || strings.Contains(noMap, "chown -R 1000:1000") {
		t.Errorf("unmapped owner without uid_map: %s", noMap)
	}

	other := ownerWarning("/config", 1000, 1000, syscall.EPERM, "", "")
	if strings.Contains(other, "This is normal on NFS with root_squash, SMB and read-only mounts") {
		t.Errorf("still blames NFS/SMB for everything: %s", other)
	}
	for _, want := range []string{"PUID/PGID", "podman unshare chown"} {
		if !strings.Contains(other, want) {
			t.Errorf("generic warning lacks %q: %s", want, other)
		}
	}
	if strings.Contains(other, ":Z") {
		t.Errorf(":Z only when SELinux is on: %s", other)
	}
	if se := ownerWarning("/config", 1000, 1000, syscall.EPERM, "", "container_t"); !strings.Contains(se, ":Z") {
		t.Errorf("SELinux on: should mention :Z on /config: %s", se)
	}
}
