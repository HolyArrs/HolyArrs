package main

import (
	"flag"
	"fmt"
	"io"
	"sort"

	"github.com/HolyArrs/holyarrs/app/backend/internal/backups"
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
	bm, unlock, err := backups.OpenDir(*dir, version)
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
	name, kept, err := backups.PutBack(bm, name)
	if err != nil {
		return fail(err)
	}
	fmt.Fprintf(out, "restored %s\nthe data it replaced is kept in %s\nstart HolyArrs again (docker restart <container>)\n", name, kept)
	return 0
}
