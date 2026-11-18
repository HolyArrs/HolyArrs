package updates

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// Launcher runs first at every start. It picks the program to run: a
// downloaded version on the volume when every check passes, else the
// image's own. It never uses the network, so a start never waits on it.
type Launcher struct {
	ConfigDir    string
	ImageVersion string
	// Keys check the downloaded program's signature (nil: Pinned()).
	Keys []PublicKey
	// Getenv reads the environment (nil: os.Getenv).
	Getenv func(string) string
	// Exec replaces this process with path, keeping its PID (nil: the
	// system's exec). It returns only on failure.
	Exec func(path string, argv, env []string) error
	// Now is the clock (nil: time.Now).
	Now func() time.Time
}

// Launch runs a downloaded version when it may; it returns when the image's
// own program should run (and then the caller simply goes on).
func (l *Launcher) Launch() {
	if l.getenv(envFromVolume) == "1" {
		return
	}
	cur, ok := l.Pick()
	if !ok {
		return
	}
	exe, err := os.Executable()
	if err != nil {
		exe = "/proc/self/exe"
	}
	env := append(withoutEnv(os.Environ(), envFromVolume, envImageBin, envImageVersion), envFromVolume+"=1", envImageBin+"="+exe, envImageVersion+"="+l.ImageVersion)
	err = l.exec(cur.Path, append([]string{cur.Path}, os.Args[1:]...), env)
	// Only reached when the program could not start (noexec, wrong
	// processor, missing exec bit): the image version runs instead.
	log.Printf("updates: could not start the downloaded %s, running the image's %s: %v", cur.Version, l.ImageVersion, err)
	f := l.folder()
	f.remove(pendingFile)
	f.setStatus(func(s *LaunchStatus) { s.Source, s.ExecError = "image", err.Error() })
}

// Pick decides which downloaded version to run, rolling back any that fails
// a check. ok is false when the image's own program runs.
func (l *Launcher) Pick() (Installed, bool) {
	f := l.folder()
	if err := os.MkdirAll(string(f), 0o750); err != nil {
		return Installed{}, false
	}
	f.clearTemps()
	if NoUpdater || strings.ToLower(strings.TrimSpace(l.getenv(EnvMode))) == "off" {
		f.setStatus(func(s *LaunchStatus) { s.Source, s.Off = "image", true })
		return Installed{}, false
	}
	f.setStatus(func(s *LaunchStatus) { s.Off = false })
	for range 3 {
		var cur Installed
		if !f.read(currentFile, &cur) {
			f.setStatus(func(s *LaunchStatus) { s.Source = "image" })
			return Installed{}, false
		}
		if !Newer(cur.Version, l.ImageVersion) {
			log.Printf("updates: the image's %s is as new as the downloaded %s, so the image runs and old downloads are removed", l.ImageVersion, cur.Version)
			for _, n := range []string{currentFile, previousFile, pendingFile} {
				f.remove(n)
			}
			f.keepOnly()
			_ = os.RemoveAll(f.path(dbCopyFolder))
			f.setStatus(func(s *LaunchStatus) { s.Source = "image" })
			return Installed{}, false
		}
		if why := l.refuse(f, cur); why != "" {
			l.revert(f, cur.Version, why)
			continue
		}
		var p pending
		if f.read(pendingFile, &p) && p.Version == cur.Version && l.now().Sub(p.Since) < pendingTTL {
			// A release that changes the database gets no second try on
			// half-changed data: one failed start goes back at once.
			if p.Starts >= 2 || (p.Migrates && p.Starts >= 1) {
				l.revert(f, cur.Version, "it did not start properly")
				continue
			}
			p.Starts++
		} else {
			p = pending{Version: cur.Version, Starts: 1, Migrates: cur.Migrates}
		}
		p.Since = l.now()
		if err := f.write(pendingFile, p); err != nil {
			return Installed{}, false
		}
		f.setStatus(func(s *LaunchStatus) { s.Source, s.ExecError = "update", "" })
		return cur, true
	}
	return Installed{}, false
}

// refuse says why cur may not run, or "".
func (l *Launcher) refuse(f folder, cur Installed) string {
	if slices.Contains(f.skip(), cur.Version) {
		return "it is on the skip list"
	}
	if kept, ok := keptList(f, l.keys(), l.now()); ok && slices.Contains(kept.Revoked, cur.Version) {
		return "it was withdrawn"
	}
	if want := filepath.Join(string(f), cur.Version, binaryName); filepath.Clean(cur.Path) != want {
		return "its file is not where it belongs"
	}
	data, err := os.ReadFile(cur.Path)
	if err != nil {
		return "its file is missing"
	}
	sum := sha256.Sum256(data)
	if hex.EncodeToString(sum[:]) != cur.SHA256 {
		return "its file changed"
	}
	sig, err := os.ReadFile(cur.Path + SigSuffix)
	if err != nil {
		return "its signature is missing"
	}
	if _, err := Verify(data, sig, l.keys()); err != nil {
		return "its signature is not valid"
	}
	return ""
}

// revert points the launcher back at the previous version (or the image),
// restores the database copy when one was made, and skips bad from now on.
func (l *Launcher) revert(f folder, bad, why string) {
	f.addSkip(bad)
	_ = os.RemoveAll(f.path(bad))
	to := "the image's " + l.ImageVersion
	var prev Installed
	if f.read(previousFile, &prev) && prev.Version != "" && prev.Version != bad {
		_ = f.write(currentFile, prev)
		to = prev.Version
	} else {
		f.remove(currentFile)
	}
	f.remove(previousFile)
	// The copy is only put back when it was made for the version being
	// left; an older one stays for going back further.
	if copyFor(f) == bad {
		if err := RestoreCopy(l.ConfigDir); err != nil {
			log.Printf("updates: could not put the database copy back: %v", err)
		}
	}
	f.remove(pendingFile)
	now := l.now()
	msg := fmt.Sprintf("%s -> %s: %s", bad, to, why)
	log.Printf("updates: went back from %s", msg)
	f.setStatus(func(s *LaunchStatus) { s.Rollback, s.RollbackAt = msg, &now })
}

// RestoreCopy puts the database copy made before an update back in place
// and removes it. The databases it replaces move to
// backups/before-restore-<time> first, so changes made since the update
// are never lost. No copy is fine: nothing changes.
func RestoreCopy(configDir string) error {
	dir := filepath.Join(configDir, AppFolder, dbCopyFolder)
	if _, err := os.Stat(filepath.Join(dir, database.MainFile)); err != nil {
		return nil
	}
	keep := filepath.Join(configDir, "backups", "before-restore-"+time.Now().UTC().Format("20060102T150405.000Z"))
	for _, name := range []string{database.MainFile, database.EventsFile} {
		src := filepath.Join(dir, name)
		if _, err := os.Stat(src); err != nil {
			continue
		}
		dst := filepath.Join(configDir, name)
		for _, suffix := range []string{"", "-wal", "-shm"} {
			if _, err := os.Lstat(dst + suffix); err != nil {
				continue
			}
			if err := os.MkdirAll(keep, 0o750); err != nil {
				return err
			}
			if err := os.Rename(dst+suffix, filepath.Join(keep, name+suffix)); err != nil {
				return err
			}
		}
		if err := copyAtomic(src, dst); err != nil {
			return err
		}
	}
	if _, err := os.Stat(keep); err == nil {
		log.Printf("updates: the database from before going back is kept in %s", keep)
	}
	return os.RemoveAll(dir)
}

func copyAtomic(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	tmp := dst + tmpSuffix
	out, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	if err := out.Sync(); err != nil {
		out.Close()
		os.Remove(tmp)
		return err
	}
	out.Close()
	if err := os.Rename(tmp, dst); err != nil {
		os.Remove(tmp)
		return err
	}
	syncDir(filepath.Dir(dst))
	return nil
}

// keptList is the last accepted update list, checked again.
func keptList(f folder, keys []PublicKey, now time.Time) (*List, bool) {
	data, err := os.ReadFile(f.path(keptListFile))
	if err != nil || len(keys) == 0 {
		return nil, false
	}
	sig, err := os.ReadFile(f.path(keptListFile + SigSuffix))
	if err != nil {
		return nil, false
	}
	l, err := Parse(data, sig, keys, now, 0)
	return l, err == nil
}

func (l *Launcher) folder() folder { return folder(filepath.Join(l.ConfigDir, AppFolder)) }

func (l *Launcher) keys() []PublicKey {
	if l.Keys != nil {
		return l.Keys
	}
	return Pinned()
}

func (l *Launcher) getenv(k string) string {
	if l.Getenv != nil {
		return l.Getenv(k)
	}
	return os.Getenv(k)
}

func (l *Launcher) exec(path string, argv, env []string) error {
	if l.Exec != nil {
		return l.Exec(path, argv, env)
	}
	return execProgram(path, argv, env)
}

func (l *Launcher) now() time.Time {
	if l.Now != nil {
		return l.Now()
	}
	return time.Now()
}

func withoutEnv(env []string, names ...string) []string {
	out := make([]string, 0, len(env))
	for _, e := range env {
		k, _, _ := strings.Cut(e, "=")
		if !slices.Contains(names, k) {
			out = append(out, e)
		}
	}
	return out
}
