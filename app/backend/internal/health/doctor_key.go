package health

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// masterKeyChecks reads the master key (HOLYARRS_MASTER_KEY_FILE or holyarrs.key
// in the config folder) and checks it opens the saved secrets. It shows
// only the key's fingerprint, never the key, and never makes a key.
func masterKeyChecks(r *Report, dir string) {
	const s = "Master key"
	if dir == "" {
		return
	}
	p := os.Getenv(secrets.EnvKeyFile)
	named := p != ""
	if !named {
		p = filepath.Join(dir, secrets.KeyFile)
	}
	saved, serr := secrets.SavedIn(dir)
	st, err := os.Stat(p)
	if errors.Is(err, fs.ErrNotExist) {
		// A named key file must exist: only the config folder's default is
		// made on first start.
		if named {
			r.add(s, StatusFail, p+" (named in "+secrets.EnvKeyFile+") is missing; HolyArrs will not start without it",
				"put the key file back at "+p+" or unset "+secrets.EnvKeyFile)
			return
		}
		if saved {
			r.add(s, StatusFail, p+" is missing, but the database holds saved secrets that only that key can open",
				"put the key file (or your key kit renamed to "+secrets.KeyFile+") back at "+p+", or name it in "+secrets.EnvKeyFile)
		} else {
			r.add(s, StatusOK, "no key yet; HolyArrs makes one on first start", "")
		}
		return
	}
	if err != nil {
		r.add(s, StatusFail, p+": cannot read the key file: "+err.Error(), "on the host, check the file's owner and mode: chown PUID:PGID and chmod 600 on it")
		return
	}
	if st.IsDir() {
		// Docker makes an empty folder when the mounted host file is missing.
		r.add(s, StatusFail, p+" is a folder, not the key file",
			"remove that folder on the host and put the key file at that path; check the host side of the volume for typos")
		return
	}
	key, err := secrets.LoadKey("", p)
	var pe *fs.PathError
	if errors.As(err, &pe) && errors.Is(err, fs.ErrPermission) {
		// A good key the app's user may not open is not damaged: the owner
		// or mode is wrong, often after a copy as root on the host.
		owner := ownerOf(st)
		if owner == "" {
			owner = "unknown"
		}
		r.add(s, StatusFail, p+" cannot be read by HolyArrs (owner "+owner+", mode "+st.Mode().Perm().String()+"): "+pe.Err.Error(),
			"on the host, give the key file to your PUID and PGID and make it private: chown PUID:PGID "+filepath.Base(p)+" and chmod 600 "+filepath.Base(p)+" (use your own numbers and the file's real path)")
		return
	}
	if errors.As(err, &pe) {
		r.add(s, StatusFail, p+": cannot read the key file: "+pe.Err.Error(), "check the disk or share that holds it, then run this again")
		return
	}
	if err != nil {
		r.add(s, StatusFail, p+" is damaged: it does not hold a 32-byte key",
			"put back a good copy of the key file, or your key kit renamed to "+secrets.KeyFile)
		return
	}
	fp := secrets.Fingerprint(key)
	switch {
	case serr != nil:
		r.add(s, StatusWarn, "key fingerprint "+fp+"; could not read the database to check it: "+serr.Error(), "")
	case !saved:
		r.add(s, StatusOK, "key fingerprint "+fp+" (no saved secrets yet)", "")
	default:
		if err := secrets.CheckFile(filepath.Join(dir, database.MainFile), key); errors.Is(err, secrets.ErrWrongKey) {
			r.add(s, StatusFail, "the key in "+p+" (fingerprint "+fp+") does not match this database",
				"put back the key file that belongs to this database; a new key cannot open the saved secrets")
		} else if err != nil {
			r.add(s, StatusWarn, "key fingerprint "+fp+"; could not check it against the database: "+err.Error(), "")
		} else {
			r.add(s, StatusOK, "key fingerprint "+fp+" matches the saved secrets", "")
		}
	}
	if runtime.GOOS != "windows" && st.Mode().Perm()&0o077 != 0 {
		r.add(s, StatusWarn, p+" can be read by other users (mode "+st.Mode().Perm().String()+")", "on the host, run chown PUID:PGID and chmod 600 on "+filepath.Base(p)+" (use your own numbers and the file's real path)")
	}
}
