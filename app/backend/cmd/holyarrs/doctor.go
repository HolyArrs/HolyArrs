package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
)

// doctorCmd runs "holyarrs doctor [-config dir] [-save-path dir] [-json]":
// the checks people would otherwise open a shell for, as the app's own
// user (it goes through the same PUID/PGID switch as reset-owner). It only
// reads the database, so it works while the server runs. Exit 0 when no
// check fails, 1 otherwise, 2 for bad flags.
func doctorCmd(args []string, out, errOut io.Writer) int {
	fs := flag.NewFlagSet("holyarrs doctor", flag.ContinueOnError)
	fs.SetOutput(errOut)
	dir := fs.String("config", "/config", "the config folder")
	save := fs.String("save-path", env("HOLYARRS_SAVE_PATH", ""), "where the Downloader saves")
	asJSON := fs.Bool("json", false, "print the report as JSON")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if fs.NArg() > 0 {
		fmt.Fprintf(errOut, "doctor: unexpected arguments: %v\n", fs.Args())
		return 2
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	r := health.RunDoctor(ctx, health.DoctorOptions{Version: version, ConfigDir: *dir, SavePath: *save})
	if *asJSON {
		_, _ = out.Write(r.JSON())
	} else {
		_, _ = io.WriteString(out, r.Text())
	}
	if r.Failed() {
		return 1
	}
	return 0
}
