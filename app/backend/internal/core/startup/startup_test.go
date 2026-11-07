package startup

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
	ro := ownerWarning("/config", 1000, 1000, syscall.EROFS, 1000, "", "", "")
	if !strings.Contains(ro, ":ro") || strings.Contains(ro, "chown") {
		t.Errorf("read-only mount: %s", ro)
	}

	// EINVAL: PUID/PGID itself is not mapped in this container; a host
	// chown cannot fix that, picking mapped IDs does.
	unmapped := ownerWarning("/config", 1000, 1000, syscall.EINVAL, 1000, "0 100000 65536\n", "0 100000 65536\n", "")
	if !strings.Contains(unmapped, "PUID/PGID 1000:1000 is not mapped inside this container") || strings.Contains(unmapped, "chown -R") {
		t.Errorf("unmapped PUID/PGID: %s", unmapped)
	}

	// EPERM on files owned by nobody (65534) under a shifted uid_map: the
	// files belong to a host user this container can't see. The host-side
	// numbers come from uid_map for the user and gid_map for the group.
	nobody := ownerWarning("/config", 1000, 1000, syscall.EPERM, 65534, "0 100000 65536\n", "0 200000 65536\n", "")
	if !strings.Contains(nobody, "chown -R 101000:201000") {
		t.Errorf("nobody-owned folder under a uid_map: %s", nobody)
	}
	// No gid_map, an identity map or a normal owner: no numbers.
	for name, w := range map[string]string{
		"no gid_map": ownerWarning("/config", 1000, 1000, syscall.EPERM, 65534, "0 100000 65536\n", "", ""),
		"identity":   ownerWarning("/config", 1000, 1000, syscall.EPERM, 65534, "0 0 4294967295\n", "0 0 4294967295\n", ""),
		"owner 0":    ownerWarning("/config", 1000, 1000, syscall.EPERM, 0, "0 100000 65536\n", "0 100000 65536\n", ""),
	} {
		if strings.Contains(w, "on the folder mounted at") {
			t.Errorf("%s: should not print host numbers: %s", name, w)
		}
	}

	other := ownerWarning("/config", 1000, 1000, syscall.EPERM, 1000, "", "", "")
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
	if se := ownerWarning("/config", 1000, 1000, syscall.EPERM, 1000, "", "", "container_t"); !strings.Contains(se, ":Z") {
		t.Errorf("SELinux on: should mention :Z on /config: %s", se)
	}
}
