package main

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"errors"
	"html"
	"io/fs"
	"log"
	"net"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/backups"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/files"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/secrets"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
	"github.com/HolyArrs/holyarrs/app/backend/internal/health"
)

// swapHandler lets the listener answer before the app is ready: a small
// page while the data is opened (and updated), then the real API.
type swapHandler struct{ h atomic.Pointer[http.Handler] }

func (s *swapHandler) set(h http.Handler) { s.h.Store(&h) }

func (s *swapHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	(*s.h.Load()).ServeHTTP(w, r)
}

// startingHandler answers while the data is opened: healthz says the app
// is alive ("updating" once updating is set: an update of the data may
// take a while, "starting" before), readyz and every page say not yet.
func startingHandler(updating *atomic.Bool) http.Handler {
	starting := holdingPage(http.StatusOK, `{"state":"starting"}`, "HolyArrs is starting, back soon.\n")
	busy := holdingPage(http.StatusOK, `{"state":"updating"}`, "HolyArrs is updating its data, back soon.\n")
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if updating != nil && updating.Load() {
			busy.ServeHTTP(w, r)
			return
		}
		starting.ServeHTTP(w, r)
	})
}

// goBack is how to return to the data from before the update.
const newMachine = " Moving to a new machine? See \"Move to a new machine\" in the HolyArrs docs, in docs/run/backups.md."

// keyRecheck is how often a held key file problem is checked again.
var keyRecheck = 10 * time.Second

// keyNeedsPerson reports whether a master key error is about the key file
// (missing, wrong, cannot be read), so the start holds a plain page instead
// of exiting into a restart loop.
func keyNeedsPerson(err error) bool {
	return errors.Is(err, secrets.ErrKeyMissing) || errors.Is(err, secrets.ErrWrongKey) || errors.Is(err, secrets.ErrKeyUnreadable)
}

const goBack = " To go back on Docker, run `docker exec <container> /holyarrs restore --pre-update`, then `docker restart <container>`. Without Docker: stop HolyArrs, run holyarrs restore --pre-update, start it again."

// failedHandler answers when the data could not be opened for a reason a
// person has to fix; healthz reports it too.
func failedHandler(msg string) http.Handler {
	return holdingPage(http.StatusServiceUnavailable, `{"state":"failed"}`, "HolyArrs did not start: "+msg+"\n")
}

// publicReason says what went wrong without paths or other details, since
// the page is open to anyone; the log keeps the full error.
func publicReason(err error) string {
	var nd *database.NewerDataError
	var ns *database.NetShareError
	switch {
	case errors.As(err, &nd):
		return "this data is from a newer HolyArrs. Run that version again, or restore a backup." + goBack
	case errors.As(err, &ns):
		return "the config folder is on a shared or network folder (" + ns.Kind + "). " + files.ShareFix(ns.Kind)
	case errors.Is(err, database.ErrInMemory):
		return "the config folder is in memory (tmpfs or ramfs), so everything in it would be lost at the next restart. Put /config on a disk (on Unraid: /mnt/<pool>/appdata/holyarrs, and check that the pool exists), or set " + files.EnvConfigInMemory + "=yes to keep it in memory anyway, then restart."
	case errors.Is(err, database.ErrNotWritable) && (errors.Is(err, syscall.ENOSPC) || errors.Is(err, syscall.EDQUOT)):
		return "the disk that holds the config folder is full or over its quota. Free some space and restart. Nothing was changed."
	case errors.Is(err, database.ErrNotWritable):
		return "the config folder cannot be written. Check that the /config volume is not mounted read-only (:ro) and that its files belong to PUID/PGID. When the container starts as root (the default), HolyArrs already tried to give them to PUID/PGID before this page showed, so a plain restart will not help: on a NAS share (NFS or SMB) fix the owner or the share's permissions on the NAS, or set PUID/PGID to the user that owns the folder there; a folder on the host can be fixed with chown -R. With a user: setting HolyArrs does not change owners; remove it to let HolyArrs fix them at the next start. Then restart. Nothing was changed."
	case errors.Is(err, database.ErrNoSpace):
		return "not enough free disk space to update safely. Free some space and restart (or set " + database.SkipSpaceCheckEnv + "=yes to skip this check). Nothing was changed."
	case errors.Is(err, database.ErrBackup):
		return "the backup before updating failed, so nothing was changed. Check that the backups folder can be written, see the log, and restart."
	case errors.Is(err, database.ErrMigration):
		return "the update of the data failed and the step that failed was undone. You can run the version you had before again (change the image tag back); if it says the data is newer, its page offers to put back the data from before the update." + goBack
	case errors.Is(err, secrets.ErrKeyMissing):
		return "the master key file is missing. Put " + secrets.KeyFile + " (or your key kit renamed to " + secrets.KeyFile + ") back in the config folder, then restart. Nothing was changed." + newMachine
	case errors.Is(err, secrets.ErrWrongKey):
		return "the master key does not match this database. Put back the key file that belongs to it, then restart. Nothing was changed." + newMachine
	case errors.Is(err, secrets.ErrKeyIsFolder):
		return "the master key path is a folder, not the key file. Docker makes an empty folder there when the host file it should mount did not exist. Remove that folder on the host, put the key file at that path (check the host side of the volume for typos), then restart. Nothing was changed."
	case errors.Is(err, secrets.ErrKeyUnreadable) && errors.Is(err, fs.ErrNotExist):
		return "the master key file named in " + secrets.EnvKeyFile + " is not there. Check that the file is mounted at that path, or remove " + secrets.EnvKeyFile + " to use " + secrets.KeyFile + " in the config folder, then restart. Nothing was changed."
	case errors.Is(err, secrets.ErrKeyUnreadable) && errors.Is(err, fs.ErrPermission):
		return "the master key file cannot be read by HolyArrs: its owner or mode is wrong. On the host, give it to your PUID and PGID and make it private: chown PUID:PGID " + secrets.KeyFile + " and chmod 600 " + secrets.KeyFile + " (use your own numbers and the file's real path), then restart. Nothing was changed."
	case errors.Is(err, secrets.ErrKeyUnreadable):
		return "the master key file cannot be read: it does not hold a key. Put back a good copy of " + secrets.KeyFile + " (or your key kit renamed to " + secrets.KeyFile + "), then restart. Nothing was changed."
	case errors.Is(err, database.ErrCheckFailed):
		return "the data check after updating failed. You can run the version you had before again (change the image tag back); its page offers to put back the data from before the update." + goBack
	}
	return "see the log for details." + goBack
}

func holdingPage(healthCode int, health, page string) http.Handler {
	m := http.NewServeMux()
	m.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(healthCode)
		_, _ = w.Write([]byte(health + "\n"))
	})
	m.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Retry-After", "10")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(page))
	})
	return m
}

// refuseEarly shows the refusal page for a config folder that cannot be
// written, found before the store opens (the log folder, the lock file),
// until ctx ends. Exiting would only restart into the same refusal.
func refuseEarly(ctx context.Context, c config, err error) error {
	log.Printf("holyarrs: not started: %v", err)
	ln, lerr := net.Listen("tcp", c.Addr)
	if lerr != nil {
		return err
	}
	srv := newServer(c.Addr, webserver.UnderURLBase(c.URLBase, failedHandler(publicReason(err))))
	go func() { _ = srv.Serve(ln) }()
	<-ctx.Done()
	_ = srv.Close()
	return err
}

// needsPerson reports whether a database.Open error is one the app should
// keep showing (newer data, no space, no backup, failed check, a config
// folder on a share, in memory or not writable) instead of exiting into a
// restart loop.
func needsPerson(err error) bool {
	var nd *database.NewerDataError
	return errors.As(err, &nd) || errors.Is(err, database.ErrNoSpace) || errors.Is(err, database.ErrBackup) || errors.Is(err, database.ErrCheckFailed) || errors.Is(err, database.ErrMigration) ||
		errors.Is(err, database.ErrNetworkShare) || errors.Is(err, database.ErrInMemory) || errors.Is(err, database.ErrNotWritable)
}

// unmappedConfigWarning is the boxed start-up warning when /config is not
// mapped to the host (kind from health.UnmappedConfig), or "" when it is.
// Start-up never refuses for it.
func unmappedConfigWarning(dir, kind string) string {
	if kind == "" {
		return ""
	}
	line := strings.Repeat("=", 72)
	return line + "\nWARNING: " + health.UnmappedText(dir, kind) + ".\nFix: " + health.UnmappedFix(dir) + ".\n" + line
}

// maxCodeTries is how many wrong codes the go-back form takes before it
// refuses until the next start (which logs a new code).
const maxCodeTries = 10

// codeAlphabet leaves out letters and digits that are easy to mix up.
const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789"

// newCode makes a one-time code like K7PQX-2MZRA (50 bits).
func newCode() string {
	b := make([]byte, 10)
	_, _ = rand.Read(b)
	for i := range b {
		b[i] = codeAlphabet[int(b[i])%len(codeAlphabet)]
	}
	return string(b[:5]) + "-" + string(b[5:])
}

// normCode drops spaces and dashes and upper-cases, so a code typed
// loosely still matches.
func normCode(s string) string {
	return strings.Map(func(r rune) rune {
		switch {
		case r == '-' || r == ' ' || r == '\t':
			return -1
		case r >= 'a' && r <= 'z':
			return r - 'a' + 'A'
		}
		return r
	}, s)
}

// goBackBackup finds the pre-update backup this build can put back, under
// the config folder's lock (raw copies an update left are made into
// backups first). ok is false when there is none, or the folder is busy.
func goBackBackup(dir string) (backups.Info, bool) {
	bm, unlock, err := backups.OpenDir(dir, version)
	if err != nil {
		log.Printf("holyarrs: cannot look for a pre-update backup: %v", err)
		return backups.Info{}, false
	}
	defer unlock()
	b, err := bm.NewestPreUpdate()
	if err != nil {
		return backups.Info{}, false
	}
	return b, true
}

// refusedHandler is the page shown when the data could not be opened. When
// this build is older than the data and a pre-update backup is there, the
// page offers to put that backup back (what holyarrs restore --pre-update
// does). The page is open to anyone and no accounts can be read from data
// this build refuses, so the button needs a one-time code that is written
// only to the log (docker logs): whoever can read it controls the
// container anyway. done is closed once the data is back, so the app
// stops and starts again on it.
func refusedHandler(dir string, err error, done chan<- struct{}) http.Handler {
	msg := publicReason(err)
	plain := failedHandler(msg)
	var nd *database.NewerDataError
	if !errors.As(err, &nd) {
		return plain
	}
	b, ok := goBackBackup(dir)
	if !ok {
		return plain
	}
	code := newCode()
	log.Printf("holyarrs: the page can put back the data from before the update (%s); to confirm there, type this one-time code: %s", b.Name, code)
	var (
		mu    sync.Mutex
		tries int
		over  bool
	)
	about := b.Detail
	if about == "" {
		about = "taken before the data was updated"
	}
	about += ", " + b.CreatedAt.UTC().Format("2006-01-02 15:04 UTC")
	page := func(w http.ResponseWriter, status int, body string) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`<!doctype html><html lang="en"><head><meta charset="utf-8"><meta name="viewport" content="width=device-width, initial-scale=1"><title>HolyArrs did not start</title>` +
			`<style>body{font:16px/1.5 system-ui,sans-serif;max-width:40rem;margin:2rem auto;padding:0 16px;background:#111;color:#eee}code{background:#222;padding:0 .3em;word-break:break-all}input,button{font:inherit;padding:.5em .7em;margin:.3em 0;max-width:100%}button{cursor:pointer}.bad{color:#f99}</style></head><body>` +
			body + `</body></html>`))
	}
	form := func(w http.ResponseWriter, status int, note string) {
		if note != "" {
			note = `<p class="bad">` + html.EscapeString(note) + `</p>`
		}
		page(w, status, `<h1>HolyArrs did not start</h1><p>`+html.EscapeString(msg)+`</p>`+
			`<h2>Put back the data from before the update</h2>`+
			`<p>This HolyArrs is older than its data. It can put back the copy kept from before the update (`+html.EscapeString(about)+`). Changes made since that update are not in that copy. The newer data is not deleted: it moves to the backups folder (before-restore-&lt;time&gt;), so this can be undone.</p>`+
			`<p>To confirm, type the one-time code from the log: run <code>docker logs &lt;container&gt;</code> and look for "one-time code".</p>`+note+
			`<form method="post"><label>Code <input name="code" autocomplete="off" autocapitalize="characters" spellcheck="false" required></label><br>`+
			`<button type="submit">Put back the data from before the update</button></form>`)
	}
	m := http.NewServeMux()
	m.Handle("GET /healthz", plain)
	m.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
		given := ""
		if r.ParseForm() == nil {
			given = normCode(r.PostForm.Get("code"))
		}
		mu.Lock()
		defer mu.Unlock()
		if over {
			page(w, http.StatusOK, `<h1>Done</h1><p>The data was already put back. HolyArrs is starting again; reload this page in a moment.</p>`)
			return
		}
		// A post with no code (a HolyArrs tab still open, Seerr, a script)
		// is not a try, so other apps cannot use up the tries.
		if given == "" {
			form(w, http.StatusBadRequest, "Type the code from the log.")
			return
		}
		if tries >= maxCodeTries {
			form(w, http.StatusTooManyRequests, "Too many wrong codes. Restart HolyArrs (docker restart <container>) for a new code.")
			return
		}
		if subtle.ConstantTimeCompare([]byte(given), []byte(normCode(code))) != 1 {
			tries++
			log.Printf("holyarrs: a wrong code was typed on the go-back page (%d of %d)", tries, maxCodeTries)
			form(w, http.StatusForbidden, "That code does not match the one in the log.")
			return
		}
		bm, unlock, err := backups.OpenDir(dir, version)
		if err != nil {
			log.Printf("holyarrs: putting back the data from before the update: %v", err)
			form(w, http.StatusConflict, "The data could not be put back; nothing was changed. See the log.")
			return
		}
		name, kept, err := backups.PutBack(bm, b.Name)
		unlock()
		if err != nil {
			log.Printf("holyarrs: putting back the data from before the update: %v", err)
			form(w, http.StatusInternalServerError, "The data could not be put back; nothing was changed. See the log.")
			return
		}
		over = true
		log.Printf("holyarrs: put back %s from the page; the data it replaced is kept in %s", name, kept)
		page(w, http.StatusOK, `<h1>Done</h1><p>The data from before the update is back in place. The newer data is kept in the backups folder as `+
			html.EscapeString(filepath.Base(kept))+`.</p><p>HolyArrs stops now and starts again on this data (Docker's restart policy starts it; without one, start the container). Reload this page in a moment.</p>`)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		go func() {
			time.Sleep(500 * time.Millisecond) // let the answer reach the page
			close(done)
		}()
	})
	m.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "10")
		form(w, http.StatusServiceUnavailable, "")
	})
	return m
}
