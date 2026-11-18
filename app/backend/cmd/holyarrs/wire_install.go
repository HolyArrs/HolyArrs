package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"
)

// newUpdateChecker is the update check: once a day from the release feed,
// through the network policy (public only unless the setting allows the
// local network; never cloud metadata addresses). HOLYARRS_RELEASE_FEED
// points it elsewhere (tests).
func newUpdateChecker(s *database.Store) *updates.Checker {
	uc := &updates.Checker{Store: s, Version: version}
	uc.ReleaseFeed = env("HOLYARRS_RELEASE_FEED", updates.DefaultReleaseFeed)
	uc.ClientFor = func(allowLocal bool) *http.Client {
		allow := outbound.PublicOnly
		if allowLocal {
			allow = outbound.NotMetadata
		}
		t := http.DefaultTransport.(*http.Transport).Clone()
		t.Proxy = nil
		t.DialContext = outbound.WatchFakeIP(outbound.PolicyDialer(allow).DialContext)
		return &http.Client{Transport: t, Timeout: 15 * time.Second}
	}
	uc.ListURL = updates.LabListURL()
	return uc
}

// errSwitch ends serve so main starts the launcher again in this process,
// which runs the version just installed (or the one gone back to).
var errSwitch = errors.New("switching version")

// updateRun watches this start of a downloaded version (nil in tests).
var updateRun *updates.Run

// launchUpdate runs the newest valid downloaded version in place of this
// program when there is one; it returns when the image's own runs.
func launchUpdate(c config) {
	(&updates.Launcher{ConfigDir: c.ConfigDir, ImageVersion: version}).Launch()
	updateRun = updates.StartRun(c.ConfigDir, version, updates.Watchdog, nil)
}

// wireInstaller gives the update check its installer. stop ends serve; the
// returned flag tells serve the stop is a version switch.
func wireInstaller(c config, s *database.Store, uc *updates.Checker, stop func()) *atomic.Bool {
	var switching atomic.Bool
	uc.Installer = &updates.Installer{
		ConfigDir: c.ConfigDir, Version: version,
		Snapshot: func(dir string) error {
			if _, err := os.Stat(filepath.Join(c.ConfigDir, database.MainFile)); err != nil {
				return nil
			}
			_, _, err := s.Snapshot(dir)
			return err
		},
		Busy: func() string {
			n, err := mediafiles.ImportsAtWork(context.Background(), s.Read())
			if err != nil || n == 0 {
				return ""
			}
			return fmt.Sprintf("%d imports", n)
		},
		Restart: func() {
			go func() {
				time.Sleep(500 * time.Millisecond) // let the answer reach the page
				switching.Store(true)
				stop()
			}()
		},
	}
	return &switching
}

// switchVersion starts the launcher again; it returns only on failure.
func switchVersion() {
	log.Printf("holyarrs: starting again to switch version")
	if err := updates.Reexec(); err != nil {
		log.Fatalf("holyarrs: could not start again to switch version: %v", err)
	}
}

// updateCheckCmd is the read-only dry run a new version passes before the
// switch: it opens this config folder's databases read-only and refuses data
// newer than it knows. No database yet passes.
func updateCheckCmd(args []string, errOut io.Writer) int {
	fs := flag.NewFlagSet(updates.CheckCommand, flag.ContinueOnError)
	fs.SetOutput(errOut)
	dir := fs.String("config", "/config", "config folder")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	mainV, eventsV := database.SchemaVersions()
	for _, x := range []struct {
		name string
		know int
	}{{database.MainFile, mainV}, {database.EventsFile, eventsV}} {
		p := filepath.Join(*dir, x.name)
		if _, err := os.Stat(p); err != nil {
			continue
		}
		db, err := sql.Open("sqlite", database.FileURI(p)+"?mode=ro")
		if err != nil {
			fmt.Fprintf(errOut, "%s: %v\n", x.name, err)
			return 1
		}
		have, err := database.SchemaVersion(db)
		db.Close()
		if err != nil {
			fmt.Fprintf(errOut, "%s: %v\n", x.name, err)
			return 1
		}
		if have > x.know {
			fmt.Fprintf(errOut, "%s holds data newer than this version knows (%d > %d)\n", x.name, have, x.know)
			return 3
		}
	}
	return 0
}

// markReady starts the healthy countdown once the server listens.
func markReady(ctx context.Context, addr string) {
	if updateRun == nil {
		return
	}
	updateRun.Ready(updates.HealthyAfter, func() bool {
		if ctx.Err() != nil {
			return false
		}
		c := http.Client{Timeout: 5 * time.Second}
		resp, err := c.Get("http://" + addr + "/healthz")
		if err != nil {
			return false
		}
		resp.Body.Close()
		return resp.StatusCode < 500
	})
}
