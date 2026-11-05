// Package runas sets up who HolyArrs runs as when it starts, the way
// linuxserver.io images do, without a shell: FILE__ variables, PUID and PGID,
// and UMASK.
//
// Started as root on Linux, HolyArrs fixes the owner of its config folder,
// switches to PUID:PGID (default 1000:1000) and starts itself again as that
// user. Started as any other user (Docker's user: setting, rootless, Kubernetes
// runAsUser) it keeps that user. On other systems only FILE__ is applied.
package runas

import (
	"errors"
	"fmt"
	"io/fs"
	"strconv"
	"strings"
)

// Marker is set in the environment of the process started after the switch,
// so it never tries to switch again.
const Marker = "HOLYARRS_USER_SWITCHED"

// filePrefix starts every variable whose value is read from a file.
const filePrefix = "FILE__"

// defaultID is the user and group HolyArrs switches to without PUID/PGID.
const defaultID = 1000

// LoadFileEnv sets NAME from the file named by every FILE__NAME variable in
// environ, with one trailing newline removed. Errors name the variable and the
// file, never the value.
func LoadFileEnv(environ []string, readFile func(string) ([]byte, error), setenv func(string, string) error) error {
	for _, kv := range environ {
		key, path, ok := strings.Cut(kv, "=")
		if !ok || !strings.HasPrefix(key, filePrefix) {
			continue
		}
		name := strings.TrimPrefix(key, filePrefix)
		if name == "" {
			return fmt.Errorf("%s has no variable name after %s", key, filePrefix)
		}
		b, err := readFile(path)
		if err != nil {
			return fmt.Errorf("%s: cannot read the file %q: %w", key, path, unwrapPath(err))
		}
		v := string(b)
		if strings.HasSuffix(v, "\r\n") {
			v = v[:len(v)-2]
		} else {
			v = strings.TrimSuffix(v, "\n")
		}
		if err := setenv(name, v); err != nil {
			return fmt.Errorf("%s: cannot set %s: %w", key, name, err)
		}
	}
	return nil
}

// unwrapPath drops the path from a file error, since the message already
// names it.
func unwrapPath(err error) error {
	var pe *fs.PathError
	if errors.As(err, &pe) {
		return pe.Err
	}
	return err
}

// ParseUmask reads UMASK as an octal number such as "022" or "0002".
func ParseUmask(s string) (int, error) {
	s = strings.TrimSpace(s)
	n, err := strconv.ParseUint(s, 8, 32)
	if err != nil || n > 0o777 {
		return 0, fmt.Errorf("UMASK %q is not an octal mask such as 022 or 002", s)
	}
	return int(n), nil
}

func parseID(name, s string) (int, error) {
	n, err := strconv.ParseUint(strings.TrimSpace(s), 10, 32)
	if err != nil || n == 1<<32-1 {
		return 0, fmt.Errorf("%s %q is not a number such as 1000", name, s)
	}
	return int(n), nil
}

// Action is what start-up does about the user.
type Action int

const (
	// Keep runs as the current user (not started as root).
	Keep Action = iota
	// Switch fixes the config folder's owner, switches to UID:GID and
	// starts again.
	Switch
	// StayRoot keeps running as root because PUID=0 and PGID=0 asked for it.
	StayRoot
)

// Plan is what Decide chose.
type Plan struct {
	Action   Action
	UID, GID int
	// Umask is the mask to apply, or -1 to keep the current one.
	Umask int
	// Note is one line to log, or empty.
	Note string
}

// Decide works out the plan from the effective user and group and the
// environment. switched is true in the process started after the switch.
func Decide(euid, egid int, lookup func(string) (string, bool), switched bool) (Plan, error) {
	p := Plan{UID: defaultID, GID: defaultID, Umask: -1}
	if v, ok := lookup("UMASK"); ok && strings.TrimSpace(v) != "" {
		m, err := ParseUmask(v)
		if err != nil {
			return p, err
		}
		p.Umask = m
	}
	uidSet, gidSet := false, false
	if v, ok := lookup("PUID"); ok && strings.TrimSpace(v) != "" {
		n, err := parseID("PUID", v)
		if err != nil {
			return p, err
		}
		p.UID, uidSet = n, true
	}
	if v, ok := lookup("PGID"); ok && strings.TrimSpace(v) != "" {
		n, err := parseID("PGID", v)
		if err != nil {
			return p, err
		}
		p.GID, gidSet = n, true
	}
	if euid != 0 {
		p.Action = Keep
		if (uidSet || gidSet) && !switched {
			p.Note = fmt.Sprintf("PUID/PGID are ignored: the container already runs as %d:%d (its user setting)", euid, egid)
		}
		return p, nil
	}
	if uidSet && p.UID == 0 {
		if !gidSet || p.GID != 0 {
			return p, errors.New("PUID=0 needs PGID=0 to run as root; set both, or use a non-root PUID such as 1000")
		}
		p.Action = StayRoot
		p.Note = "PUID=0 and PGID=0: running as root, as asked (not recommended; PUID/PGID such as 1000 are safer)"
		return p, nil
	}
	if switched {
		return p, fmt.Errorf("still root after switching to %d:%d; HolyArrs will not run as root", p.UID, p.GID)
	}
	p.Action = Switch
	return p, nil
}

// startMask is the umask HolyArrs runs with. Linux sets it at start-up; other
// systems keep the usual 022.
var startMask = 0o022

// Perm is perm with the start-up umask applied, which is what creating a file
// with perm gives. Use it where a mode is set with chmod, so UMASK decides.
func Perm(perm fs.FileMode) fs.FileMode {
	return perm &^ fs.FileMode(startMask)
}

// NeedsOwnerFix reports whether the config folder's owner differs from the
// user HolyArrs switches to. Only then is the folder walked, so a normal start
// costs one stat.
func NeedsOwnerFix(curUID, curGID, uid, gid int) bool {
	return curUID != uid || curGID != gid
}

// Groups is the group list after the switch: gid first, then the
// supplementary groups the process already has (Docker's group_add), without
// duplicates and without root's group 0 unless gid is 0.
func Groups(gid int, current []int) []int {
	out := []int{gid}
	for _, g := range current {
		if g == 0 && gid != 0 {
			continue
		}
		dup := false
		for _, o := range out {
			dup = dup || o == g
		}
		if !dup {
			out = append(out, g)
		}
	}
	return out
}

// SwitchFailed is the start-up error when root cannot switch user.
func SwitchFailed(uid, gid int, err error) error {
	return fmt.Errorf("started as root but could not switch to %d:%d (%v). "+
		"Add back the SETUID, SETGID and CHOWN capabilities if the container drops them (cap_drop: ALL), "+
		"or run the container as that user with user: \"%d:%d\". HolyArrs will not run as root",
		uid, gid, err, uid, gid)
}
