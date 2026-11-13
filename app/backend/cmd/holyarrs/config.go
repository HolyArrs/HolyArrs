package main

import (
	"context"
	"database/sql"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"strconv"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/downloads"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

var version = "0.0.0-skeleton"

type config struct {
	ConfigDir string
	Addr      string
	// Skeleton setup: one Source, one Downloader, one movie Library folder.
	SourceURL string
	QbitURL   string
	Movies    string
	SavePath  string
	// SafeMode starts only the core (sign-in, the Library, settings,
	// backups, health): no schedules, searches, downloads or imports.
	SafeMode bool
	// URLBase runs the app under a path such as /holyarrs (reverse proxy).
	URLBase string
	// urlBaseFlag is true when -url-base was given on the command line.
	urlBaseFlag bool
}

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// EnvSafeMode set to 1 (or true) starts HolyArrs in safe mode.
const EnvSafeMode = "HOLYARRS_SAFE_MODE"

func envOn(key string) bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(key))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

// defaultPort is the web port HolyArrs listens on unless -addr says otherwise
// (6262, Psalm 62). The health check uses the same port.
const defaultPort = "6262"

// EnvAddr sets the listen address (such as :8080) and EnvPort only its port
// (such as 8080). -addr wins over both, and EnvAddr over EnvPort.
const (
	EnvAddr = "HOLYARRS_ADDR"
	EnvPort = "HOLYARRS_PORT"
)

// envListenAddr is the listen address from EnvAddr or EnvPort, or the
// default. A bad value is an error that names the variable.
func envListenAddr() (string, error) {
	if v := strings.TrimSpace(os.Getenv(EnvAddr)); v != "" {
		_, port, err := net.SplitHostPort(v)
		if err == nil {
			err = validPort(port)
		}
		if err != nil {
			return "", fmt.Errorf("%s=%q is not a listen address such as :8080 or 0.0.0.0:8080", EnvAddr, v)
		}
		return v, nil
	}
	if v := strings.TrimSpace(os.Getenv(EnvPort)); v != "" {
		if err := validPort(v); err != nil {
			return "", fmt.Errorf("%s=%q is not a port number from 1 to 65535", EnvPort, v)
		}
		return ":" + v, nil
	}
	return ":" + defaultPort, nil
}

func validPort(s string) error {
	if n, err := strconv.Atoi(s); err != nil || n < 1 || n > 65535 {
		return fmt.Errorf("bad port %q", s)
	}
	return nil
}

func parseFlags(args []string, errOut io.Writer) (config, error) {
	fs := flag.NewFlagSet("holyarrs", flag.ContinueOnError)
	fs.SetOutput(errOut)
	var c config
	fs.StringVar(&c.ConfigDir, "config", "/config", "folder for the database and settings (must be on local disk)")
	fs.StringVar(&c.Addr, "addr", ":"+defaultPort, "address to listen on ("+EnvAddr+", or "+EnvPort+" for the port only)")
	fs.StringVar(&c.SourceURL, "source-url", env("HOLYARRS_SOURCE_URL", ""), "Torznab Source to add on start (no key)")
	fs.StringVar(&c.QbitURL, "qbittorrent-url", env("HOLYARRS_QBITTORRENT_URL", ""), "qBittorrent Web UI to add on start")
	fs.StringVar(&c.Movies, "movies", env("HOLYARRS_MOVIES", ""), "movie Library folder to add on start")
	fs.StringVar(&c.SavePath, "save-path", env("HOLYARRS_SAVE_PATH", ""), "where the Downloader saves (same path in both containers)")
	fs.BoolVar(&c.SafeMode, "safe-mode", envOn(EnvSafeMode), "start only the core: sign-in, the Library, settings, backups and health ("+EnvSafeMode+"=1)")
	fs.StringVar(&c.URLBase, "url-base", env(webserver.URLBaseEnv, ""), "serve under this path behind a reverse proxy, such as /holyarrs ("+webserver.URLBaseEnv+")")
	if err := fs.Parse(args); err != nil {
		return c, err
	}
	addrFlag := false
	fs.Visit(func(f *flag.Flag) {
		c.urlBaseFlag = c.urlBaseFlag || f.Name == "url-base"
		addrFlag = addrFlag || f.Name == "addr"
	})
	if !addrFlag {
		addr, err := envListenAddr()
		if err != nil {
			fmt.Fprintln(errOut, err)
			return c, err
		}
		c.Addr = addr
	}
	base, err := webserver.CleanURLBase(c.URLBase)
	if err != nil {
		fmt.Fprintln(errOut, "-url-base:", err)
		return c, err
	}
	c.URLBase = base
	if fs.NArg() > 0 {
		return c, fmt.Errorf("unexpected arguments: %v", fs.Args())
	}
	return c, nil
}

// ensure adds the configured Source, Downloader and Library folder once.
func ensure(ctx context.Context, s *database.Store, c config) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		if c.Movies != "" && !library.FullPath(c.Movies) {
			// Git Bash turns /data/media/movies into "C:/Program Files/Git/data/media/movies"
			// when a container is made without MSYS_NO_PATHCONV=1: a folder that can never be reached.
			log.Printf("setup: movie Library folder %q is not a full path here; not added", c.Movies)
		} else if c.Movies != "" {
			if err := mediafiles.EnsureFolder(tx, c.Movies, "movie", "Movies"); err != nil {
				return err
			}
		}
		if c.SourceURL != "" {
			if err := sources.EnsureOwn(tx, domain.Source{Name: "Lab fake indexer", Protocol: "torrent", Format: "torznab",
				URL: c.SourceURL, UseFor: domain.SourceUse{Feed: false, AutoSearch: true, HandSearch: true}, Kinds: []string{"movie"},
				Categories: map[string][]int{"movie": {2000}}, Limits: domain.SourceLimits{GapMS: 2000, SearchesPerWindow: 100, WindowSeconds: 3600},
				Priority: 25, State: "enabled", Extra: map[string]string{}, AllowLocal: true}); err != nil {
				return err
			}
		}
		if c.QbitURL != "" {
			if err := downloads.EnsureDownloader(tx, domain.Downloader{Name: "Lab qBittorrent", Adapter: "qbittorrent", Protocol: "torrent",
				URL: c.QbitURL, Category: "holyarrs", Priority: 1, PathMap: []domain.PathMap{}, AfterImport: "leave", State: "ok", AllowLocal: true}); err != nil {
				return err
			}
		}
		return nil
	})
}
