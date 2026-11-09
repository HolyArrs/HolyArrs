package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/files"
)

// options are the flags every command shares, plus seed's own.
type options struct {
	ConfigDir string
	// AppURL is where the running app would answer; the commands refuse
	// to change a config folder while it does.
	AppURL string
	// MediaRoot is the lab media folder (movies, tv, music inside it).
	MediaRoot string
	// Force allows a config folder or media root outside the lab.
	Force bool
	// InitLab marks the config folder as the lab's and does nothing else.
	InitLab bool
	// Reset removes what seed added instead of adding.
	Reset bool
	// DownloadsDir is the Downloader save folder (HOLYARRS_SAVE_PATH).
	DownloadsDir string
	// SabINI is the lab SABnzbd config file its key is read from.
	SabINI string
	// Now is the clock (tests).
	Now func() time.Time
}

func parseCommon(name string, args []string) (options, error) {
	fs := flag.NewFlagSet(name, flag.ContinueOnError)
	var o options
	fs.StringVar(&o.ConfigDir, "config", "/config", "the HolyArrs config folder (stop HolyArrs first)")
	fs.StringVar(&o.AppURL, "app-url", "http://holyarrs:6262", "where HolyArrs answers when it runs; nothing is changed while it does (empty skips the check)")
	fs.StringVar(&o.MediaRoot, "media", "/data/media", "the lab media folder: movies, tv and music are made inside it")
	fs.BoolVar(&o.Force, "force", false, "allow a media folder other than /data/media, and go on when the app address gives no clear answer (the lab marker and the app's lock are always checked)")
	fs.BoolVar(&o.InitLab, "init-lab", false, "seed: mark this config folder as the lab's ("+labMarker+") and stop; do this once, by hand")
	fs.BoolVar(&o.Reset, "reset-demo", false, "seed: remove only what an earlier seed added")
	fs.StringVar(&o.DownloadsDir, "downloads", env("HOLYARRS_SAVE_PATH", "/data/downloads/complete"), "seed: the Downloader save folder")
	fs.StringVar(&o.SabINI, "sabnzbd-ini", "", "seed: the lab SABnzbd config file (sabnzbd.ini); its key goes into the vault, never shown")
	if err := fs.Parse(args); err != nil {
		return o, err
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(os.Stderr, "unexpected arguments: %v\n", fs.Args())
		return o, errors.New("unexpected arguments")
	}
	return o, nil
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func (o options) now() time.Time {
	if o.Now != nil {
		return o.Now().UTC()
	}
	return time.Now().UTC()
}

// errRunning is returned while the app answers on AppURL.
var errRunning = errors.New("HolyArrs is running on this config folder; stop it first (docker stop holyarrs)")

// checkStopped refuses when the app answers at its address. Only a
// refused connection counts as stopped; a name that does not resolve, a
// timeout or any other error leaves it unclear and refuses unless forced.
func checkStopped(ctx context.Context, appURL string, force bool) error {
	if appURL == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(appURL, "/")+"/healthz", nil)
	if err != nil {
		return fmt.Errorf("app address %q: %w", appURL, err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		if refused(err) || force {
			return nil
		}
		return fmt.Errorf("cannot tell whether HolyArrs runs at %s (%v); stop it and pass -force, or give the right -app-url", appURL, err)
	}
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<12))
	resp.Body.Close()
	return errRunning
}

// refused reports a refused connection (ECONNREFUSED, or WSAECONNREFUSED
// on Windows).
func refused(err error) bool {
	if errors.Is(err, syscall.ECONNREFUSED) {
		return true
	}
	var en syscall.Errno
	return errors.As(err, &en) && uintptr(en) == 10061
}

// lockConfig takes the lock the app holds on its config folder while it
// runs, so the app cannot start while a command changes the folder. A
// held lock means the app runs.
func lockConfig(dir string) (func(), error) {
	unlock, err := files.LockDir(dir)
	if errors.Is(err, files.ErrDirLocked) {
		return nil, errRunning
	}
	if err != nil {
		return nil, fmt.Errorf("lock %s: %w", dir, err)
	}
	return unlock, nil
}

// labMarker is the file in a config folder that marks it as the lab's.
// Only the lab setup or an explicit "seed -init-lab" makes it.
const labMarker = "lab.marker"

// checkLab refuses folders that are not the lab: the config folder must
// hold a HolyArrs database and the lab marker, and the media folder must be
// the lab's /data/media with its movies folder. -force waives the media
// folder check only, never the database or the marker.
func checkLab(o options) error {
	if _, err := os.Stat(filepath.Join(o.ConfigDir, "holyarrs.db")); err != nil {
		return fmt.Errorf("%s has no HolyArrs database; seed only fills an existing lab copy", o.ConfigDir)
	}
	if err := checkMarker(o.ConfigDir); err != nil {
		return err
	}
	if o.Force {
		return nil
	}
	if filepath.ToSlash(filepath.Clean(o.MediaRoot)) != "/data/media" {
		return fmt.Errorf("media folder %s is not the lab's /data/media; pass -force if this really is a lab", o.MediaRoot)
	}
	if fi, err := os.Stat(filepath.Join(o.MediaRoot, "movies")); err != nil || !fi.IsDir() {
		return fmt.Errorf("%s has no movies folder; is the lab media folder mounted?", o.MediaRoot)
	}
	return nil
}

// checkMarker refuses a config folder without the lab marker.
func checkMarker(dir string) error {
	if fi, err := os.Stat(filepath.Join(dir, labMarker)); err != nil || !fi.Mode().IsRegular() {
		return fmt.Errorf("%s has no %s, so it is not known to be the lab; if it really is, mark it once with: holyarrs-demo seed -config %s -init-lab", dir, labMarker, dir)
	}
	return nil
}

// initLab marks an existing HolyArrs config folder as the lab's. It adds
// the marker file only; nothing else in the folder changes.
func initLab(o options, out io.Writer) error {
	if _, err := os.Stat(filepath.Join(o.ConfigDir, "holyarrs.db")); err != nil {
		return fmt.Errorf("%s has no HolyArrs database; only an existing lab copy is marked", o.ConfigDir)
	}
	p := filepath.Join(o.ConfigDir, labMarker)
	if err := os.WriteFile(p, []byte("This HolyArrs config folder is the lab's; holyarrs-demo may change it.\n"), 0o644); err != nil {
		return err
	}
	fmt.Fprintf(out, "marked %s as the lab\n", o.ConfigDir)
	return nil
}
