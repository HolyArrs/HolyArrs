package main

import (
	"context"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/startup"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
)

// healthcheck asks a running server for its health: 0 when it answers 200, 1
// otherwise. The image has no shell or curl, so its HEALTHCHECK uses this.
func healthcheck(base string) int {
	cl := &http.Client{Timeout: 5 * time.Second}
	resp, err := cl.Get(strings.TrimRight(base, "/") + "/healthz")
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

// healthcheckCmd runs -healthcheck. It applies FILE__ only, so
// FILE__HOLYARRS_URL_BASE and FILE__HOLYARRS_HEALTH_URL work; it never
// switches user or sets UMASK.
func healthcheckCmd() int {
	if err := startup.LoadFileEnv(os.Environ(), os.ReadFile, os.Setenv); err != nil {
		log.Printf("holyarrs: %v", err)
		return 1
	}
	// The root /healthz answers under any URL base, also one set by flag or
	// in Settings, which the probe cannot see.
	if u := os.Getenv("HOLYARRS_HEALTH_URL"); u != "" {
		return healthcheck(u)
	}
	addr, err := envListenAddr()
	if err != nil {
		log.Printf("holyarrs: %v", err)
		return 1
	}
	return healthcheck(healthBase(addr))
}

// healthBase is the address the health check calls for a listen address:
// this machine when the address names no host or every host.
func healthBase(addr string) string {
	host, port, _ := net.SplitHostPort(addr)
	if ip := net.ParseIP(host); host == "" || (ip != nil && ip.IsUnspecified()) {
		host = "127.0.0.1"
	}
	return "http://" + net.JoinHostPort(host, port)
}

// logFolderCheck logs one line on the Library and download folders, and a
// WARN line with the fix for each problem folder. It takes a few seconds at
// most: a folder that is slower (a hung network share) is skipped with a note.
func logFolderCheck(ctx context.Context, db database.Lister, savePath, configDir string) {
	sum, warns := health.DoctorStartup(ctx, db, savePath, 3*time.Second)
	log.Printf("holyarrs: %s", sum)
	for _, w := range warns {
		log.Printf("holyarrs: %s", w)
	}
	for _, w := range health.UnsafeFolderWarnings(ctx, db, configDir) {
		log.Printf("holyarrs: WARN %s", w)
	}
	if w := health.ProxyWarning(os.Getenv); w != "" {
		log.Printf("holyarrs: warning: %s (%s)", w, "proxies for indexers can be set on each Source")
	}
}

// configDirArg finds -config in the arguments of the server or a command,
// before the flags are parsed, so start-up knows which folder to give to
// PUID:PGID.
func configDirArg(args []string) string {
	dir := "/config"
	for i := 0; i < len(args); i++ {
		a := args[i]
		if a == "--" {
			break
		}
		if v, ok := strings.CutPrefix(a, "-config="); ok {
			dir = v
		} else if v, ok := strings.CutPrefix(a, "--config="); ok {
			dir = v
		} else if (a == "-config" || a == "--config") && i+1 < len(args) {
			dir = args[i+1]
			i++
		}
	}
	return dir
}

// resetOwner sets the owner's name and password from stdin ("name: ..." and
// "password: ..." lines), for when the password is lost. Every session and API
// key ends. It works while the server runs.
func resetOwner(args []string) int {
	c, err := parseFlags(args, os.Stderr)
	if err != nil {
		return 2
	}
	name, pw, err := accounts.ReadNewOwner(os.Stdin)
	if err != nil {
		log.Printf("reset-owner: %v", err)
		return 1
	}
	s, err := database.Open(c.ConfigDir, database.Options{})
	if err != nil {
		log.Printf("reset-owner: %v", err)
		return 1
	}
	defer s.Close()
	if err := accounts.ResetOwner(context.Background(), s, name, pw); err != nil {
		log.Printf("reset-owner: %v", err)
		return 1
	}
	log.Printf("reset-owner: the owner is now %q; sign in again everywhere", name)
	log.Printf("reset-owner: open invites and arr API keys were removed; review allowed hosts and local networks in Settings")
	return 0
}
