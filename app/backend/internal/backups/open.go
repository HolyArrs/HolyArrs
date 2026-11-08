package backups

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/files"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
)

// OpenDir takes the config folder's lock (refused while HolyArrs runs
// on it) and turns raw copies left by an update into pre-update backups.
// The caller calls unlock when done.
func OpenDir(dir, version string) (*Manager, func(), error) {
	if _, err := os.Stat(dir); err != nil {
		return nil, nil, fmt.Errorf("config folder: %w", err)
	}
	unlock, err := files.LockDir(dir)
	if err != nil {
		if errors.Is(err, files.ErrDirLocked) {
			return nil, nil, errors.New("HolyArrs is running on this config folder; stop it first (docker stop <container>), then run this again")
		}
		return nil, nil, err
	}
	bm := &Manager{Dir: dir, Version: version}
	if named := os.Getenv(secrets.EnvKeyFile); named != "" || fileExists(filepath.Join(dir, secrets.KeyFile)) {
		key, err := secrets.LoadKey(dir, named)
		if err != nil {
			unlock()
			return nil, nil, err
		}
		bm.CheckKey = func(p string) error { return secrets.CheckFile(p, key) }
	}
	if _, err := bm.AdoptMigrationCopies(); err != nil {
		unlock()
		return nil, nil, err
	}
	return bm, unlock, nil
}

// PutBack checks the backup name (the newest pre-update backup this build
// can read when name is "") and puts it in place. The data it replaces is
// kept in backups/before-restore-<time>, returned as kept.
func PutBack(bm *Manager, name string) (string, string, error) {
	if name == "" {
		b, err := bm.NewestPreUpdate()
		if err != nil {
			return "", "", err
		}
		name = b.Name
	}
	if err := bm.StageNamed(context.Background(), name); err != nil {
		return name, "", fmt.Errorf("%s: %w", name, err)
	}
	if _, err := ApplyStaged(bm.Dir); err != nil {
		return name, "", fmt.Errorf("putting %s in place: %w (it stays staged and is tried again on the next start)", name, err)
	}
	kept := ""
	if ds, _ := filepath.Glob(filepath.Join(bm.Dir, "backups", "before-restore-*")); len(ds) > 0 {
		sort.Strings(ds)
		kept = ds[len(ds)-1]
	}
	return name, kept, nil
}

func fileExists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
