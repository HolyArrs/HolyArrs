package doctor

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"

	"github.com/CoderLuii/holyarrs/internal/secrets"
	"github.com/CoderLuii/holyarrs/internal/store"
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
			r.add(s, Fail, p+" (named in "+secrets.EnvKeyFile+") is missing; HolyArrs will not start without it",
				"put the key file back at "+p+" or unset "+secrets.EnvKeyFile)
			return
		}
		if saved {
			r.add(s, Fail, p+" is missing, but the database holds saved secrets that only that key can open",
				"put the key file (or your key kit renamed to "+secrets.KeyFile+") back at "+p+", or name it in "+secrets.EnvKeyFile)
		} else {
			r.add(s, OK, "no key yet; HolyArrs makes one on first start", "")
		}
		return
	}
	if err != nil {
		r.add(s, Fail, p+": cannot read the key file: "+err.Error(), "check the file's owner and mode")
		return
	}
	key, err := secrets.LoadKey("", p)
	if err != nil {
		r.add(s, Fail, p+" is damaged: it does not hold a 32-byte key",
			"put back a good copy of the key file, or your key kit renamed to "+secrets.KeyFile)
		return
	}
	fp := secrets.Fingerprint(key)
	switch {
	case serr != nil:
		r.add(s, Warn, "key fingerprint "+fp+"; could not read the database to check it: "+serr.Error(), "")
	case !saved:
		r.add(s, OK, "key fingerprint "+fp+" (no saved secrets yet)", "")
	default:
		if err := secrets.CheckFile(filepath.Join(dir, store.MainFile), key); errors.Is(err, secrets.ErrWrongKey) {
			r.add(s, Fail, "the key in "+p+" (fingerprint "+fp+") does not match this database",
				"put back the key file that belongs to this database; a new key cannot open the saved secrets")
		} else if err != nil {
			r.add(s, Warn, "key fingerprint "+fp+"; could not check it against the database: "+err.Error(), "")
		} else {
			r.add(s, OK, "key fingerprint "+fp+" matches the saved secrets", "")
		}
	}
	if runtime.GOOS != "windows" && st.Mode().Perm()&0o077 != 0 {
		r.add(s, Warn, p+" can be read by other users (mode "+st.Mode().Perm().String()+")", "chmod 600 "+p)
	}
}
