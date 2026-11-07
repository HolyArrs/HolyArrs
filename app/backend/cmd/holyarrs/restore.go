package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/HolyArrs/holyarrs/app/backend/internal/backup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/files"
	"github.com/HolyArrs/holyarrs/app/backend/internal/secrets"
)

const restoreUsage = "usage:\n" +
	"  holyarrs restore [-config DIR] --list           list the backups\n" +
	"  holyarrs restore [-config DIR] --pre-update     go back to the newest pre-update backup\n" +
	"  holyarrs restore [-config DIR] NAME             go back to the backup NAME\n" +
	"Docker: run `docker exec <container> /holyarrs restore --pre-update`, then\n" +
	"`docker restart <container>`. Without Docker: stop HolyArrs, run\n" +
	"holyarrs restore --pre-update, start it again. The data it replaces is kept\n" +
	"in backups/before-restore-<time>, so a restore can be undone.\n"

// restoreCmd is the way back when the app cannot start (newer data, a failed
// update): it lists backups, or checks one and puts it in place, keeping the
// current data aside.
func restoreCmd(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("holyarrs restore", flag.ContinueOnError)
	fs.SetOutput(errOut)
	fs.Usage = func() { fmt.Fprint(errOut, restoreUsage) }
	dir := fs.String("config", "/config", "the config folder")
	list := fs.Bool("list", false, "list the backups")
	pre := fs.Bool("pre-update", false, "restore the newest pre-update backup")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	name := fs.Arg(0)
	if fs.NArg() > 1 || (*list && (*pre || name != "")) || (*pre && name != "") || (!*list && !*pre && name == "") {
		fs.Usage()
		return 2
	}
	fail := func(err error) int {
		fmt.Fprintf(errOut, "restore: %v\n", err)
		return 1
	}
	bm, unlock, err := openBackups(*dir)
	if err != nil {
		return fail(err)
	}
	defer unlock()
	if *list {
		all, err := bm.List()
		if err != nil {
			return fail(err)
		}
		if len(all) == 0 {
			fmt.Fprintln(out, "no backups")
			return 0
		}
		sort.SliceStable(all, func(i, j int) bool { return all[i].CreatedAt.After(all[j].CreatedAt) })
		for _, b := range all {
			fmt.Fprintf(out, "%s  %-10s  %s  %s\n", b.CreatedAt.UTC().Format("2006-01-02 15:04 UTC"), b.Kind, b.Name, b.Detail)
		}
		return 0
	}
	name, kept, err := putBack(bm, name)
	if err != nil {
		return fail(err)
	}
	fmt.Fprintf(out, "restored %s\nthe data it replaced is kept in %s\nstart HolyArrs again (docker restart <container>)\n", name, kept)
	return 0
}

// openBackups takes the config folder's lock (refused while HolyArrs runs
// on it) and turns raw copies left by an update into pre-update backups.
// The caller calls unlock when done.
func openBackups(dir string) (*backup.Manager, func(), error) {
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
	bm := &backup.Manager{Dir: dir, Version: version}
	if named := os.Getenv(secrets.EnvKeyFile); named != "" || exists(filepath.Join(dir, secrets.KeyFile)) {
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

// putBack checks the backup name (the newest pre-update backup this build
// can read when name is "") and puts it in place. The data it replaces is
// kept in backups/before-restore-<time>, returned as kept.
func putBack(bm *backup.Manager, name string) (string, string, error) {
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
	if _, err := backup.ApplyStaged(bm.Dir); err != nil {
		return name, "", fmt.Errorf("putting %s in place: %w (it stays staged and is tried again on the next start)", name, err)
	}
	kept := ""
	if ds, _ := filepath.Glob(filepath.Join(bm.Dir, "backups", "before-restore-*")); len(ds) > 0 {
		sort.Strings(ds)
		kept = ds[len(ds)-1]
	}
	return name, kept, nil
}

func exists(p string) bool {
	_, err := os.Stat(p)
	return err == nil
}
