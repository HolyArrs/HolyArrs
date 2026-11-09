package files

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"
	"syscall"
	"testing"
)

// Mountinfo lines as Docker, Docker Desktop, Unraid and Podman write them
// (fields: id parent major:minor root mountpoint options ... - fstype source superoptions).
const (
	rootOverlay = "1180 1100 0:52 / / rw,relatime - overlay overlay rw,lowerdir=/var/lib/docker/overlay2/l/A,upperdir=/x,workdir=/y"
	hex64       = "3f1e9a0c5b7d2e4f6a8b0c1d2e3f4a5b6c7d8e9f0a1b2c3d4e5f6a7b8c9d0e1f"
)

func mi(lines ...string) string { return strings.Join(lines, "\n") + "\n" }

func TestFSTypePicksTheLongestMountAndOnlyTheType(t *testing.T) {
	cases := []struct {
		name, info, dir, want string
	}{
		{"docker desktop windows bind", mi(rootOverlay, "1201 1180 0:60 /run/desktop/mnt/host/c/cfg /config rw,noatime - 9p drvfs rw,dirsync,aname=drvfs;path=C:\\;uid=0"), "/config", "9p"},
		{"unraid user share", mi(rootOverlay, "1201 1180 0:44 / /config rw - fuse.shfs shfs rw,user_id=0"), "/config", "fuse.shfs"},
		{"mergerfs", mi(rootOverlay, "1201 1180 0:44 /appdata /config rw - fuse.mergerfs pool rw"), "/config", "fuse.mergerfs"},
		{"folder inside a mount", mi(rootOverlay, "1201 1180 8:1 /srv/data /data rw - ext4 /dev/sda1 rw"), "/data/config", "ext4"},
		{"longest match wins", mi(rootOverlay, "1201 1180 8:1 /srv /data rw - ext4 /dev/sda1 rw", "1202 1201 0:70 / /data/config rw - virtiofs share rw"), "/data/config", "virtiofs"},
		{"prefix is not a parent", mi(rootOverlay, "1201 1180 8:1 /srv /conf rw - ext4 /dev/sda1 rw"), "/config", "overlay"},
		{"escaped mount point", mi(rootOverlay, `1201 1180 8:1 /x /my\040config rw - xfs /dev/sdb1 rw`), "/my config", "xfs"},
		{"odd characters are not passed on", mi(rootOverlay, "1201 1180 8:1 /x /config rw - ext4<script> /dev/sda1 rw"), "/config", ""},
		{"too long is not passed on", mi(rootOverlay, "1201 1180 8:1 /x /config rw - "+strings.Repeat("a", 40)+" /dev/sda1 rw"), "/config", ""},
		{"no separator", mi("1201 1180 8:1 /x /config rw ext4 /dev/sda1 rw"), "/config", ""},
		{"empty", "", "/config", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := FSType(c.info, c.dir); got != c.want {
				t.Fatalf("FSType = %q, want %q", got, c.want)
			}
		})
	}
}

// FSType never hands out the mount source or its options (they can hold
// host paths, share names or credentials).
func TestFSTypeNeverLeaksSourceOrOptions(t *testing.T) {
	info := mi(rootOverlay, "1201 1180 0:44 / /config rw - cifs //nas/private-share rw,vers=3.0,addr=10.0.0.9")
	got := FSType(info, "/config")
	if got != "cifs" {
		t.Fatalf("FSType = %q, want cifs", got)
	}
}

func TestFamilyHasOneFixPerFilesystem(t *testing.T) {
	cases := []struct {
		fstype       string
		labelHas     []string
		fixHas       []string
		fixHasNot    []string
		emptyAllowed bool
	}{
		{fstype: "9p", fixHas: []string{"named volume", "holyarrs-config:/config"}, fixHasNot: []string{"cache"}},
		{fstype: "fuse.grpcfuse", fixHas: []string{"named volume", "holyarrs-config:/config"}},
		{fstype: "osxfs", fixHas: []string{"named volume"}},
		{fstype: "fakeowner", fixHas: []string{"named volume"}},
		{fstype: "fuse.shfs", labelHas: []string{"Unraid"}, fixHas: []string{"/mnt/<pool>/appdata", "Exclusive"}, fixHasNot: []string{"cache"}},
		{fstype: "fuse.mergerfs", labelHas: []string{"mergerfs"}, fixHas: []string{"member disk"}},
		{fstype: "fuseblk", fixHas: []string{"ext4"}},
		{fstype: "fuse.ntfs-3g", fixHas: []string{"ext4"}},
		{fstype: "exfat", fixHas: []string{"ext4"}},
		{fstype: "virtiofs", fixHas: []string{"VM's own disk"}},
		{fstype: "fuse.rclone", labelHas: []string{"remote"}},
		{fstype: "fuse.sshfs", labelHas: []string{"remote"}},
		{fstype: "fuse.s3fs", labelHas: []string{"remote"}},
		{fstype: "fuse.somethingnew", labelHas: []string{"FUSE", "Unraid /mnt/user", "mergerfs"}},
		{fstype: "fuse", labelHas: []string{"FUSE"}},
		{fstype: "nfs", fixHas: []string{"local disk"}},
		{fstype: "nfs4", fixHas: []string{"local disk"}},
		{fstype: "cifs", fixHas: []string{"local disk"}},
		{fstype: "smb3", fixHas: []string{"local disk"}},
	}
	for _, c := range cases {
		t.Run(c.fstype, func(t *testing.T) {
			label, fix := Family(c.fstype)
			if label == "" || fix == "" {
				t.Fatalf("Family(%q) = %q, %q; want both set", c.fstype, label, fix)
			}
			for _, w := range c.labelHas {
				if !strings.Contains(label, w) {
					t.Errorf("label %q lacks %q", label, w)
				}
			}
			for _, w := range c.fixHas {
				if !strings.Contains(fix, w) {
					t.Errorf("fix %q lacks %q", fix, w)
				}
			}
			for _, w := range c.fixHasNot {
				if strings.Contains(strings.ToLower(fix), w) {
					t.Errorf("fix %q should not say %q", fix, w)
				}
			}
		})
	}
	// Local disks have no family: nothing to fix.
	for _, local := range []string{"ext4", "xfs", "btrfs", "zfs", "overlay", ""} {
		if label, fix := Family(local); label != "" || fix != "" {
			t.Errorf("Family(%q) = %q, %q; want empty for a local disk", local, label, fix)
		}
	}
}

func TestWhyNamesTheErrno(t *testing.T) {
	cases := []struct {
		err  error
		want string
	}{
		{syscall.EROFS, "read-only"},
		{syscall.EACCES, "no write permission"},
		{syscall.EPERM, "no write permission"},
		{syscall.ENOSPC, "full"},
		{syscall.EDQUOT, "full"},
		{syscall.EINVAL, "not mapped"},
		{&fs.PathError{Op: "open", Path: "/config/x", Err: syscall.EROFS}, "read-only"},
		{fmt.Errorf("creating: %w", &fs.PathError{Op: "open", Path: "/x", Err: syscall.EACCES}), "no write permission"},
	}
	for _, c := range cases {
		if got := Why(c.err); !strings.Contains(got, c.want) {
			t.Errorf("Why(%v) = %q, want it to contain %q", c.err, got, c.want)
		}
	}
	if got := Why(errors.New("something else")); got != "" {
		t.Errorf("Why(plain error) = %q, want empty", got)
	}
	if got := Why(nil); got != "" {
		t.Errorf("Why(nil) = %q, want empty", got)
	}
}

func TestHostIDReadsEveryRange(t *testing.T) {
	cases := []struct {
		name, uidMap string
		id, want     int
		ok           bool
	}{
		{"rootless podman", "         0       1000          1\n         1     100000      65536\n", 1000, 100999, true},
		{"rootless podman root", "0 1000 1\n1 100000 65536\n", 0, 1000, true},
		{"identity map", "0 0 4294967295\n", 1000, 1000, true},
		{"single range", "0 100000 65536\n", 1000, 101000, true},
		{"outside every range", "0 100000 65536\n", 70000, 0, false},
		{"two ranges cover it", "0 100000 65536\n1000 200000 10\n", 1000, 0, false},
		{"empty", "", 1000, 0, false},
		{"garbage", "a b c\n", 1000, 0, false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := HostID(c.uidMap, c.id)
			if ok != c.ok || (ok && got != c.want) {
				t.Fatalf("HostID = %d, %v; want %d, %v", got, ok, c.want, c.ok)
			}
		})
	}
}

func TestSELinuxTypeIgnoresAppArmorAndCategories(t *testing.T) {
	cases := map[string]string{
		"system_u:system_r:container_t:s0:c123,c456\x00": "container_t",
		"system_u:system_r:spc_t:s0\n":                   "spc_t",
		"u:r:container_t:s0":                             "container_t",
		"docker-default (enforce)\n":                     "",
		"unconfined\n":                                   "",
		"":                                               "",
	}
	for in, want := range cases {
		if got := SELinuxType(in); got != want {
			t.Errorf("SELinuxType(%q) = %q, want %q", in, got, want)
		}
	}
}

// Unmapped tells whether /config is a forgotten volume: an anonymous
// Docker or Podman volume, or no mount at all (it sits in the container's
// own layer). Anything it is unsure about stays silent ("").
func TestUnmappedConfig(t *testing.T) {
	cases := []struct {
		name, info, dir, want string
	}{
		{"anonymous docker volume", mi(rootOverlay, "1201 1180 8:1 /var/lib/docker/volumes/"+hex64+"/_data /config rw,relatime master:1 - ext4 /dev/sda1 rw"), "/config", "anonymous"},
		{"anonymous volume, docker data on its own disk", mi(rootOverlay, "1201 1180 8:17 /volumes/"+hex64+"/_data /config rw - xfs /dev/sdb1 rw"), "/config", "anonymous"},
		{"anonymous rootless podman volume", mi(rootOverlay, "1201 1180 0:40 /home/u/.local/share/containers/storage/volumes/"+hex64+"/_data /config rw - btrfs /dev/nvme0n1p2 rw"), "/config", "anonymous"},
		{"named volume", mi(rootOverlay, "1201 1180 8:1 /var/lib/docker/volumes/holyarrs-config/_data /config rw - ext4 /dev/sda1 rw"), "/config", ""},
		{"named volume that looks like hex but is short", mi(rootOverlay, "1201 1180 8:1 /var/lib/docker/volumes/3f1e9a0c/_data /config rw - ext4 /dev/sda1 rw"), "/config", ""},
		{"bind mount", mi(rootOverlay, "1201 1180 8:1 /srv/appdata/holyarrs /config rw - ext4 /dev/sda1 rw"), "/config", ""},
		{"parent bind mount", mi(rootOverlay, "1201 1180 8:1 /srv/data /data rw - ext4 /dev/sda1 rw"), "/data/config", ""},
		{"not mapped at all", mi(rootOverlay, "1201 1180 8:1 /srv/media /media rw - ext4 /dev/sda1 rw"), "/config", "none"},
		{"bare metal root", mi("25 1 8:1 / / rw,relatime - ext4 /dev/sda1 rw"), "/var/lib/holyarrs", ""},
		{"unreadable mountinfo", "", "/config", ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := Unmapped(c.info, c.dir); got != c.want {
				t.Fatalf("Unmapped = %q, want %q", got, c.want)
			}
		})
	}
}
