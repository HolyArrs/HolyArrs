package main

import (
	"context"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/backups"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
)

// doctor copies the config folder, runs every health check once on the
// copy and explains each result in plain words, then lists failed Jobs,
// unreachable Sources, Downloaders and media servers, and recent problem
// Events. The real config folder is only read.
func doctor(ctx context.Context, o options, out io.Writer) error {
	tmp, err := os.MkdirTemp("", "holyarrs-doctor-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	if err := copyConfig(o.ConfigDir, tmp); err != nil {
		return fmt.Errorf("copying the config folder: %w", err)
	}
	s, err := database.Open(tmp, database.Options{})
	if err != nil {
		return err
	}
	defer s.Close()
	rep, err := diagnose(ctx, s, tmp, o.now())
	if err != nil {
		return err
	}
	for _, l := range rep {
		fmt.Fprintln(out, l)
	}
	return nil
}

// copyConfig copies the databases, the backups folder listing (files) and
// leaves out the key and the artwork.
func copyConfig(from, to string) error {
	return filepath.WalkDir(from, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(from, p)
		if d.IsDir() {
			if rel == "artwork" {
				return filepath.SkipDir
			}
			return os.MkdirAll(filepath.Join(to, rel), 0o750)
		}
		if strings.HasSuffix(rel, ".key") || !d.Type().IsRegular() {
			return nil
		}
		b, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(filepath.Join(to, rel), b, 0o600)
	})
}

// diagnose returns the report lines.
func diagnose(ctx context.Context, s *database.Store, dir string, now time.Time) ([]string, error) {
	bm := &backups.Manager{Store: s, Dir: dir, Version: "doctor"}
	he := &health.Engine{Store: s, Version: "doctor", Now: func() time.Time { return now },
		Backups: func(ctx context.Context) (health.Backups, error) {
			newest, every, lastErr, err := bm.State(ctx)
			return health.Backups{Newest: newest, Every: every, LastErr: lastErr}, err
		}}
	rs, err := he.Check(ctx)
	if err != nil {
		return nil, err
	}
	var out []string
	bad := 0
	out = append(out, "== Health checks (run once on a copy)")
	for _, r := range rs {
		if r.State == health.OK {
			out = append(out, fmt.Sprintf("   ok       %-15s %s", r.Kind, r.Subject))
			continue
		}
		bad++
		out = append(out, fmt.Sprintf("   %-8s %-15s %s", r.State, r.Kind, r.Message))
		if why := explain(r); why != "" {
			out = append(out, "            what to do: "+why)
		}
	}
	db := s.Read()
	out = append(out, "== Sources and Downloaders (can they be reached from here?)")
	for _, q := range []string{"SELECT 'Source', name, url FROM sources WHERE state != 'disabled'", "SELECT 'Downloader', name, url FROM downloaders WHERE removed = 0 AND state != 'disabled'", "SELECT 'Media server', name, url FROM media_servers WHERE enabled = 1"} {
		rows, err := db.QueryContext(ctx, q)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var kind, name, link string
			if rows.Scan(&kind, &name, &link) != nil {
				continue
			}
			state := "answers"
			if !reachable(link) {
				state = "does not answer (is its lab container stopped?)"
				bad++
			}
			out = append(out, fmt.Sprintf("   %-12s %-24s %s", kind, name, state))
		}
		rows.Close()
	}
	out = append(out, "== Jobs that failed in the last 7 days")
	rows, err := db.QueryContext(ctx, "SELECT kind, COALESCE(error, ''), count(*) FROM jobs WHERE state = 'failed' AND updated_at > ? GROUP BY kind, error ORDER BY 3 DESC LIMIT 20",
		database.FormatTime(now.Add(-7*24*time.Hour)))
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var kind, msg string
		var n int
		if rows.Scan(&kind, &msg, &n) == nil {
			bad++
			out = append(out, fmt.Sprintf("   %dx %s: %s", n, kind, msg))
		}
	}
	rows.Close()
	out = append(out, "== Problem Events in the last 7 days")
	evs, _, err := s.Events(ctx, database.EventFilter{Limit: 500})
	if err != nil {
		return nil, err
	}
	seen := map[string]int{}
	var order []string
	for _, e := range evs {
		if e.At.Before(now.Add(-7*24*time.Hour)) || !problemEvent[e.Kind] {
			continue
		}
		k := e.Kind + ": " + e.Summary
		if seen[k] == 0 {
			order = append(order, k)
		}
		seen[k]++
	}
	for _, k := range order {
		out = append(out, fmt.Sprintf("   %dx %s", seen[k], k))
	}
	out = append(out, fmt.Sprintf("== %d things need a look", bad))
	return out, nil
}

var problemEvent = map[string]bool{"health_broken": true, "alert_failed": true, "library_folder_unreachable": true,
	"media_server_failed": true, "free_space_low": true, "title_dropped": true, "file_missing": true, "download_failed": true}

// explain says in plain words what to do about one result.
func explain(r health.Result) string {
	switch r.Kind {
	case "source":
		return "start the lab (python research/lab/labctl.py up) or pause the Source in Sources"
	case "downloader":
		return "start the lab qBittorrent or SABnzbd container, or pause the Downloader in Downloaders"
	case "library_folder":
		if strings.Contains(r.Message, "hardlinked") {
			return "nothing breaks: the lab container mounts downloads and media separately, so imports are copied; mount research/lab once as /data to hardlink"
		}
		return "check the media mount of the holyarrs container"
	case "disk_space":
		return "free space on the disk that holds the Library folder"
	case "backups":
		return "make a backup in This app > Backups (a scheduled one follows on its own)"
	case "update":
		return "clear the update check link in This app if there is no update server"
	}
	return ""
}

// reachable dials the host of a link for up to two seconds.
func reachable(link string) bool {
	u, err := url.Parse(link)
	if err != nil || u.Host == "" {
		return false
	}
	host := u.Host
	if u.Port() == "" {
		if u.Scheme == "https" {
			host = net.JoinHostPort(u.Hostname(), "443")
		} else {
			host = net.JoinHostPort(u.Hostname(), "80")
		}
	}
	c, err := net.DialTimeout("tcp", host, 2*time.Second)
	if err != nil {
		return false
	}
	c.Close()
	return true
}
