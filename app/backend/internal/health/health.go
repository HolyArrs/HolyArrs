package health

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

// KindCheck is the Job kind that runs the checks (scheduled every 5 minutes).
const KindCheck = "health_check"

// Results.
const (
	OK      = "ok"
	Warning = "warning"
	Error   = "error"
)

// Settings in the this_app group.
const (
	settingGuard = "health_flap_guard"   // runs in a row before an Alert (default 2)
	settingLowMB = "health_low_space_mb" // warn under this much free space (default 1024)
)

// Result is one check's answer about one thing.
type Result struct {
	Key     string // unique, e.g. downloader:3
	Kind    string // library_folder, disk_space, source, downloader, backups, update
	Subject string // what it is about, e.g. the Downloader's name
	State   string // ok, warning, error
	Message string // plain words; empty when ok
}

// Backups tells the backups check about the newest backup.
type Backups struct {
	Newest  time.Time     // zero when there is none
	Every   time.Duration // how often a scheduled backup is made
	LastErr string        // the last failed scheduled backup, if any
	// Started says the first scheduled pass since start has finished (a
	// fresh install has no backup until then, which is not a problem).
	Started bool
	// KeyPrint is the current master key's fingerprint; "" when the key
	// is a named file (HOLYARRS_MASTER_KEY_FILE) the person looks after.
	KeyPrint string
	// KeyKitPrint is the fingerprint of the last key kit downloaded.
	KeyKitPrint string
}

// UpdateState is the update check's last answer as the checks need it.
type UpdateState struct {
	On        bool
	Feed      string
	CheckedAt *time.Time
	Every     time.Duration
	Latest    string
	Newer     bool
}

// Engine runs the checks.
type Engine struct {
	Store *database.Store
	// Version is this program's version, for the update check.
	Version string
	// Backups reads the newest backup (nil skips the check).
	Backups func(ctx context.Context) (Backups, error)
	// UpdateState reads the update check's last answer without asking
	// anywhere (nil: no update check).
	UpdateState func(ctx context.Context) UpdateState
	// UpdateCheck runs the update check when it is due; ran is false when
	// the check is off or has nowhere to ask (nil: no update check).
	UpdateCheck func(ctx context.Context) (latest string, newer, ran bool)
	// FreeSpace reads free bytes (nil means importer.FreeSpace; -1 unknown).
	FreeSpace func(dir string) int64
	// Outside reads the outside-call breaker states (nil means the process
	// registry, outbound.States).
	Outside func() []outbound.State
	// Now is the clock (nil means time.Now).
	Now func() time.Time
	// Publish tells the web app the checks changed (may be nil).
	Publish func(topic string, data any)
	// Trailers reports the trailer lookups (nil skips the check).
	Trailers func(ctx context.Context) Trailers
	// ConfigUnmapped is "anonymous" or "none" when /config is not mapped to
	// the host (an Error until it is); "" adds no check.
	ConfigUnmapped string
	// ConfigDir is the config folder; its disk is checked for free space
	// ("" adds no check).
	ConfigDir string

	mu  sync.Mutex
	low map[string]int64 // folder -> bytes an import needed but could not get
	run sync.Mutex       // one run at a time
	wg  sync.WaitGroup   // runs started by LowSpace
}

func (e *Engine) now() time.Time {
	if e.Now != nil {
		return e.Now().UTC()
	}
	return time.Now().UTC()
}

// Register lets the Job queue run the checks.
func (e *Engine) Register(q *jobs.Queue) {
	q.Handle(KindCheck, func(ctx context.Context, r *jobs.Run) error {
		cs, err := e.Run(ctx)
		if err == nil {
			bad := 0
			for _, c := range cs {
				if c.State != OK {
					bad++
				}
			}
			r.SetResult(map[string]any{"checks": len(cs), "not_ok": bad})
		}
		return err
	})
}

// LowSpace is the importer's hook: an import into dir was refused because
// only free bytes were left and need were needed. The disk check reports it
// at once and until the space is there.
func (e *Engine) LowSpace(ctx context.Context, dir string, free, need int64) {
	e.mu.Lock()
	if e.low == nil {
		e.low = map[string]int64{}
	}
	e.low[dir] = need
	e.mu.Unlock()
	e.wg.Add(1)
	go func() {
		defer e.wg.Done()
		// An error here waits for the next scheduled run.
		jobs.Guard("health", func() { _, _ = e.Run(context.WithoutCancel(ctx)) })
	}()
}

// Wait waits for the runs LowSpace started.
func (e *Engine) Wait() { e.wg.Wait() }

func (e *Engine) setting(ctx context.Context, key string) string {
	var v string
	_ = e.Store.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", key).Scan(&v)
	return v
}

func (e *Engine) intSetting(ctx context.Context, key string, def, lo, hi int) int {
	n, err := strconv.Atoi(e.setting(ctx, key))
	if err != nil || n < lo || n > hi {
		return def
	}
	return n
}

// update is the update check's row: a newer release is a warning; no
// release yet, a failed check or the check turned off are not.
func (e *Engine) update(ctx context.Context) ([]Result, error) {
	if e.UpdateCheck == nil {
		return nil, nil
	}
	latest, newer, ran := e.UpdateCheck(ctx)
	if !ran {
		return nil, nil
	}
	r := Result{Key: "update", Kind: "update", Subject: "HolyArrs " + e.Version, State: OK}
	if newer {
		r.State, r.Message = Warning, "HolyArrs "+latest+" is out (this is "+e.Version+")"
	}
	return []Result{r}, nil
}

// Check runs every check once and returns the results, without storing.
func (e *Engine) Check(ctx context.Context) ([]Result, error) {
	var out []Result
	for _, f := range []func(context.Context) ([]Result, error){e.configFolder, e.folders, e.sources, e.downloaders, e.waiting, e.backups, e.trailers, e.update} {
		rs, err := f(ctx)
		if err != nil {
			return nil, err
		}
		out = append(out, rs...)
	}
	return out, nil
}

// Probe reports whether Run would make a meaningful stored health change.
// Stable confirmed checks do not need their checked_at timestamp or streak
// rewritten. An expired update cache still needs the durable Job because it
// may perform the daily network check and persist its answer.
func (e *Engine) Probe(ctx context.Context, now time.Time) (bool, any, error) {
	e.run.Lock()
	defer e.run.Unlock()

	var info UpdateState
	if e.UpdateState != nil {
		info = e.UpdateState(ctx)
	}
	if info.On && info.Feed != "" && (info.CheckedAt == nil || !now.UTC().Before(info.CheckedAt.Add(info.Every))) {
		return true, nil, nil
	}
	var rs []Result
	for _, f := range []func(context.Context) ([]Result, error){e.configFolder, e.folders, e.sources, e.downloaders, e.waiting, e.backups, e.trailers} {
		got, err := f(ctx)
		if err != nil {
			return true, nil, err
		}
		rs = append(rs, got...)
	}
	if info.On && info.Feed != "" {
		r := Result{Key: "update", Kind: "update", Subject: "HolyArrs " + e.Version, State: OK}
		if info.Newer {
			r.State, r.Message = Warning, "HolyArrs "+info.Latest+" is out (this is "+e.Version+")"
		}
		rs = append(rs, r)
	}
	old, err := database.Select[domain.HealthCheck](ctx, e.Store.Read(), "health_checks", "1=1")
	if err != nil {
		return true, nil, err
	}
	byKey := make(map[string]domain.HealthCheck, len(old))
	for _, c := range old {
		byKey[c.Key] = c
	}
	for _, r := range rs {
		c, ok := byKey[r.Key]
		if !ok || c.Kind != r.Kind || c.Subject != r.Subject || c.State != r.State || c.Message != r.Message || c.State != c.Confirmed {
			return true, nil, nil
		}
		delete(byKey, r.Key)
	}
	return len(byKey) != 0, nil, nil
}

// Run checks everything, stores the results and writes an Event for every
// break or recovery that held for the flap guard's number of runs.
func (e *Engine) Run(ctx context.Context) ([]domain.HealthCheck, error) {
	e.run.Lock()
	defer e.run.Unlock()
	rs, err := e.Check(ctx)
	if err != nil {
		return nil, err
	}
	guard := e.intSetting(ctx, settingGuard, 2, 1, 20)
	now := e.now()
	var events []*domain.Event
	err = e.Store.Write(ctx, func(tx *sql.Tx) error {
		events = nil
		old, err := database.Select[domain.HealthCheck](ctx, tx, "health_checks", "1=1")
		if err != nil {
			return err
		}
		byKey := map[string]domain.HealthCheck{}
		for _, c := range old {
			byKey[c.Key] = c
		}
		seen := map[string]bool{}
		for _, r := range rs {
			seen[r.Key] = true
			c, had := byKey[r.Key]
			if !had {
				c = domain.HealthCheck{Key: r.Key, Since: now, Confirmed: OK}
			}
			if had && c.State == r.State {
				c.Streak++
			} else {
				c.Streak, c.Since = 1, now
			}
			c.Kind, c.Subject, c.State, c.Message, c.CheckedAt = r.Kind, r.Subject, r.State, r.Message, now
			if c.State != c.Confirmed && c.Streak >= guard {
				ev := &domain.Event{Kind: domain.EventHealthBroken, Summary: c.Message,
					Data:     map[string]any{"check": c.Key, "kind": c.Kind, "subject": c.Subject, "state": c.State, "message": c.Message, "was": c.Confirmed},
					GroupKey: "health:" + c.Key}
				if c.State == OK {
					ev.Kind, ev.Summary = domain.EventHealthRecovered, "Fixed: "+label(c.Kind)+" "+c.Subject
				}
				events = append(events, ev)
				c.Confirmed = c.State
			}
			if had {
				if _, err := tx.Exec(`UPDATE health_checks SET kind = ?, subject = ?, state = ?, message = ?, since = ?, checked_at = ?, streak = ?, confirmed = ? WHERE id = ?`,
					c.Kind, c.Subject, c.State, c.Message, database.FormatTime(c.Since), database.FormatTime(c.CheckedAt), c.Streak, c.Confirmed, c.ID); err != nil {
					return err
				}
			} else if _, err := tx.Exec(`INSERT INTO health_checks (check_key, kind, subject, state, message, since, checked_at, streak, confirmed) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
				c.Key, c.Kind, c.Subject, c.State, c.Message, database.FormatTime(c.Since), database.FormatTime(c.CheckedAt), c.Streak, c.Confirmed); err != nil {
				return err
			}
		}
		// Things that are gone (a removed Downloader) drop their check. A
		// trailers check stays while nothing reports trailers yet (at start,
		// before the reader is wired), so a kept warning is not dropped and
		// announced again.
		for k, c := range byKey {
			if !seen[k] && (c.Kind != "trailers" || e.Trailers != nil) {
				if _, err := tx.Exec("DELETE FROM health_checks WHERE id = ?", c.ID); err != nil {
					return err
				}
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, ev := range events {
		if err := e.Store.AddEvent(ctx, ev); err != nil {
			return nil, err
		}
		if e.Publish != nil {
			e.Publish("event", ev)
		}
	}
	cs, err := List(ctx, e.Store)
	if err == nil && e.Publish != nil {
		e.Publish("health", map[string]any{"checks": len(cs)})
	}
	return cs, err
}

// List returns the stored results, worst first.
func List(ctx context.Context, s *database.Store) ([]domain.HealthCheck, error) {
	cs, err := database.Select[domain.HealthCheck](ctx, s.Read(), "health_checks", "1=1")
	if err != nil {
		return nil, err
	}
	rank := map[string]int{Error: 0, Warning: 1, OK: 2}
	sort.SliceStable(cs, func(i, j int) bool {
		if rank[cs[i].State] != rank[cs[j].State] {
			return rank[cs[i].State] < rank[cs[j].State]
		}
		return cs[i].Key < cs[j].Key
	})
	if cs == nil {
		cs = []domain.HealthCheck{}
	}
	return cs, nil
}

func label(kind string) string {
	switch kind {
	case "library_folder":
		return "Library folder"
	case "disk_space":
		return "free space on"
	case "source":
		return "Source"
	case "downloader":
		return "Downloader"
	case "waiting_downloads":
		return "downloads waiting for their files"
	case "backups":
		return "backups"
	case "update":
		return "update check"
	case "trailers":
		return "trailer lookups"
	case "config_folder":
		return "config folder"
	case "config_disk":
		return "free space on"
	}
	return kind
}

// TrailerFailLimit is how many different titles (or music videos) must
// fail, with no good lookup in between, before the trailers check warns.
// A title counts once, however many of its trailers were tried.
const TrailerFailLimit = 5

// Trailers is what the trailers check reads: the state of HolyArrs's own
// YouTube reader.
type Trailers struct {
	On bool // trailers are looked up at all
	// Failed is how many different titles or music videos failed since the
	// last good lookup, for any reason but one about that video alone (age, sign-in, a content
	// warning); kept across restarts.
	Failed   int
	LastGood time.Time // the last lookup that found addresses (zero: none yet)
	// LastReason is the reason of the last failure Failed counts
	// (metadata.TrailerReason), "" when none.
	LastReason string
}

// readerTrouble says a failure in plain words: YouTube's door, or videos
// that played before no longer being found.
func readerTrouble(reason string) string {
	switch metadata.TrailerReason(reason) {
	case metadata.TrailerUnavailable, metadata.TrailerUnplayable:
		return "Trailers are not being found; a HolyArrs update may be needed"
	case metadata.TrailerBotCheck:
		return "YouTube asks to confirm you're not a bot"
	case metadata.TrailerNoAddress:
		return "YouTube changed how its videos are reached; a HolyArrs update will fix it"
	case metadata.TrailerRefused:
		return "YouTube turned the lookups away"
	case metadata.TrailerNetwork:
		return "YouTube could not be reached"
	}
	return "Trailer lookups are failing"
}

// trailers names the reader's last good lookup and, while lookups fail,
// why and how many different videos failed; it warns after
// TrailerFailLimit of them, which is how a change at YouTube shows. One
// video failing again and again, or a video that is plainly about itself
// (age, sign-in, a content warning), does not add up to a warning.
func (e *Engine) trailers(ctx context.Context) ([]Result, error) {
	if e.Trailers == nil {
		return nil, nil
	}
	t := e.Trailers(ctx)
	if !t.On {
		return nil, nil
	}
	r := Result{Key: "trailers", Kind: "trailers", Subject: "Trailer reader", State: OK}
	if !t.LastGood.IsZero() {
		r.Subject += ", last good lookup " + t.LastGood.UTC().Format("2 Jan 15:04 UTC")
	}
	if t.Failed > 0 {
		in := strconv.Itoa(t.Failed) + " different videos failed in a row"
		if t.Failed == 1 {
			in = "the last lookup failed"
		}
		// Below the limit, a trailer that will not play (removed, blocked)
		// is nothing to report: that happens all the time. YouTube turning
		// the reader away is named at once.
		video := metadata.TrailerReason(t.LastReason).VideoReason()
		if t.Failed >= TrailerFailLimit || !video {
			r.Message = readerTrouble(t.LastReason) + " (" + in + ")"
		}
		if t.Failed >= TrailerFailLimit {
			r.State = Warning
		}
	}
	return []Result{r}, nil
}

func mb(n int64) string { return strconv.FormatInt(n>>20, 10) + " MB" }

// folders checks each movie Library folder: it must be there, and its disk
// must have room.
func (e *Engine) folders(ctx context.Context) ([]Result, error) {
	lfs, err := database.Select[mediafiles.LibraryFolder](ctx, e.Store.Read(), "library_folders", "1=1 ORDER BY id")
	if err != nil {
		return nil, err
	}
	rules, _ := importer.ReadFileRules(ctx, e.Store.Read())
	floor := int64(rules.MinFreeMB) << 20
	warn := int64(e.intSetting(ctx, settingLowMB, 1024, 0, 1<<24)) << 20
	free := e.FreeSpace
	if free == nil {
		free = importer.FreeSpace
	}
	var out []Result
	for _, lf := range lfs {
		id := strconv.FormatInt(lf.ID, 10)
		name := lf.Path
		r := Result{Key: "library_folder:" + id, Kind: "library_folder", Subject: name, State: OK}
		st, err := os.Stat(lf.Path)
		switch {
		case err != nil:
			r.State, r.Message = Error, "Library folder "+name+" cannot be reached (is the disk or share mounted?)"
		case !st.IsDir():
			r.State, r.Message = Error, "Library folder "+name+" is not a folder"
		case lf.Health == "read_only":
			r.State, r.Message = Error, "HolyArrs cannot write to the Library folder "+name
		case lf.Health == "no_hardlink" && !lf.CopiesExpected:
			r.State, r.Message = Warning, "Library folder "+name+" is on another disk or mount than the downloads, so imports are copied, not hardlinked (each file takes its space twice while it seeds); to hardlink, put the downloads and this folder on one disk and one mount"
		}
		out = append(out, r)
		if err != nil {
			continue
		}
		d := Result{Key: "disk_space:" + id, Kind: "disk_space", Subject: name, State: OK}
		n := free(lf.Path)
		need, refused := e.refusedIn(lf.Path)
		switch {
		case n < 0:
			// unknown free space never warns
		case refused && n < need+floor:
			d.State, d.Message = Error, fmt.Sprintf("An import into %s was refused for lack of space: it needs %s and %s must stay free, only %s are free", name, mb(need), mb(floor), mb(n))
		case n < floor:
			d.State, d.Message = Error, fmt.Sprintf("Only %s are free on %s; imports there are refused under %s", mb(n), name, mb(floor))
		case n < warn:
			d.State, d.Message = Warning, fmt.Sprintf("Only %s are free on %s", mb(n), name)
		}
		if refused && n >= need+floor {
			e.mu.Lock()
			for dir := range e.low {
				if inside(dir, lf.Path) {
					delete(e.low, dir)
				}
			}
			e.mu.Unlock()
		}
		out = append(out, d)
	}
	return out, nil
}

// refusedIn is the most an import into the folder (or below it) needed
// but could not get.
func (e *Engine) refusedIn(folder string) (int64, bool) {
	e.mu.Lock()
	defer e.mu.Unlock()
	var need int64
	found := false
	for dir, n := range e.low {
		if inside(dir, folder) {
			need, found = max(need, n), true
		}
	}
	return need, found
}

func inside(p, folder string) bool {
	p, folder = filepath.Clean(p), filepath.Clean(folder)
	return p == folder || strings.HasPrefix(p, folder+string(filepath.Separator))
}

// sources warns about every Source HolyArrs cannot use right now.
func (e *Engine) sources(ctx context.Context) ([]Result, error) {
	ss, err := database.Select[domain.Source](ctx, e.Store.Read(), "sources", "1=1 ORDER BY id")
	if err != nil {
		return nil, err
	}
	var out []Result
	for _, s := range ss {
		h := sources.HealthOf(s)
		r := Result{Key: "source:" + strconv.FormatInt(s.ID, 10), Kind: "source", Subject: s.Name, State: OK}
		if h.State == sources.StateUnreachable || h.State == sources.StateAuthFailed {
			r.State, r.Message = Warning, "Source "+s.Name+": "+h.Text
		}
		out = append(out, r)
	}
	return out, nil
}

// downloaders warns about every Downloader that does not answer or turned
// down its sign-in. One paused by a person is fine.
func (e *Engine) downloaders(ctx context.Context) ([]Result, error) {
	ds, err := database.Select[domain.Downloader](ctx, e.Store.Read(), "downloaders", "removed = 0 ORDER BY id")
	if err != nil {
		return nil, err
	}
	var out []Result
	for _, d := range ds {
		r := Result{Key: "downloader:" + strconv.FormatInt(d.ID, 10), Kind: "downloader", Subject: d.Name, State: OK}
		why := strings.TrimSpace(d.LastError)
		switch d.State {
		case "unreachable":
			r.State, r.Message = Warning, "Downloader "+d.Name+" does not answer"
		case "auth_failed":
			r.State, r.Message = Warning, "Downloader "+d.Name+" turned down the sign-in"
		}
		if r.State != OK && why != "" {
			r.Message += ": " + why
		}
		out = append(out, r)
	}
	return out, nil
}

// waitingFor is how long downloads may wait for their files (a share not
// mounted yet) before the check turns from a warning to an error.
const waitingFor = 3 * 24 * time.Hour

// waiting counts the downloads that wait because their Downloader can't
// use the files, most often a share mounted late (late-mounts-3): a
// warning at first, an error once one has waited about 3 days. A download
// paused by a person has no reason and is not counted.
func (e *Engine) waiting(ctx context.Context) ([]Result, error) {
	ds, err := database.Select[domain.Download](ctx, e.Store.Read(), "downloads",
		"state = 'paused' AND state_reason LIKE 'the Downloader can''t use the files%'")
	if err != nil {
		return nil, err
	}
	r := Result{Key: "waiting_downloads", Kind: "waiting_downloads", Subject: "Downloads", State: OK}
	if len(ds) == 0 {
		return []Result{r}, nil
	}
	oldest := ds[0].UpdatedAt
	for _, d := range ds[1:] {
		if d.UpdatedAt.Before(oldest) {
			oldest = d.UpdatedAt
		}
	}
	r.State = Warning
	if e.now().Sub(oldest) > waitingFor {
		r.State = Error
	}
	if len(ds) == 1 {
		r.Message = "1 download waits because its Downloader can't use the files: is the share mounted? Once it is, recheck it in the Downloader (or use Check again on the Downloads page)."
	} else {
		r.Message = strconv.Itoa(len(ds)) + " downloads wait because their Downloader can't use the files: is the share mounted? Once it is, recheck them in the Downloader (or use Check again on the Downloads page)."
	}
	return []Result{r}, nil
}

// Free space on the config disk: a Warning under configLowWarn, an Error
// under configLowError. Once it is full every database write fails.
const (
	configLowWarn  = 1 << 30
	configLowError = 256 << 20
)

// configDisk checks the free space on the disk that holds the config folder
// (one statfs).
func (e *Engine) configDisk() []Result {
	if e.ConfigDir == "" {
		return nil
	}
	free := e.FreeSpace
	if free == nil {
		free = importer.FreeSpace
	}
	r := Result{Key: "config_disk", Kind: "config_disk", Subject: "Config folder", State: OK}
	n := free(e.ConfigDir)
	switch {
	case n < 0:
		// unknown free space never warns
	case n < configLowError:
		r.State, r.Message = Error, fmt.Sprintf("Only %s are free on the disk that holds the config folder. When it is full HolyArrs cannot save anything. Free some space on that disk, or move the config folder to a bigger one", mb(n))
	case n < configLowWarn:
		r.State, r.Message = Warning, fmt.Sprintf("Only %s are free on the disk that holds the config folder. Keep at least 1 GB free there so HolyArrs can save its data and backups", mb(n))
	}
	return []Result{r}
}

// configFolder is an Error while /config is not mapped to the host, since
// everything in it goes when the container is removed.
func (e *Engine) configFolder(context.Context) ([]Result, error) {
	disk := e.configDisk()
	if e.ConfigUnmapped == "" {
		return disk, nil
	}
	where := "an anonymous volume"
	if e.ConfigUnmapped == "none" {
		where = "the container itself"
	}
	return append([]Result{{Key: "config_folder", Kind: "config_folder", Subject: "Config folder", State: Error,
		Message: "/config is not mapped to a folder or named volume, so it lives in " + where + " and everything is lost when the container is removed or recreated. Stop the container, copy it out with docker cp, then map /config and copy it back in"}}, disk...), nil
}

// backups warns when the newest backup is older than two backup periods
// (and a day), or the last scheduled backup failed.
func (e *Engine) backups(ctx context.Context) ([]Result, error) {
	if e.Backups == nil {
		return nil, nil
	}
	b, err := e.Backups(ctx)
	r := Result{Key: "backups", Kind: "backups", Subject: "Backups", State: OK}
	switch {
	case err != nil:
		r.State, r.Message = Warning, "The backups folder cannot be read"
	case b.LastErr != "":
		r.State, r.Message = Error, "The last scheduled backup failed: "+b.LastErr
	case b.Newest.IsZero() && !b.Started:
		// Right after start: the first scheduled backup is still being made.
	case b.Newest.IsZero():
		r.State, r.Message = Warning, "There is no backup yet"
	case b.Every > 0 && e.now().Sub(b.Newest) > 2*b.Every+24*time.Hour:
		r.State, r.Message = Warning, "The newest backup is from "+b.Newest.UTC().Format("2006-01-02")
	}
	// Backups do not hold the master key: without a copy of it, the saved
	// passwords in a backup cannot be opened on a new machine.
	k := Result{Key: "backups:key", Kind: "backups", Subject: "Backups", State: OK}
	if err == nil && b.KeyPrint != "" && b.KeyKitPrint != b.KeyPrint {
		k.State, k.Message = Warning, "Download your key kit: Backups don't include the master key"
	}
	return []Result{r, k}, nil
}
