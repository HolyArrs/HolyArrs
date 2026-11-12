package downloads

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

var jobSuffix = regexp.MustCompile(`\.\d{1,3}$`)

// ClearFinished removes finished jobs from the Downloader that downloadID
// names. A clear that fails waits and is tried again on a later sync (see
// retryClears) until it works or the job is gone.
func (m *Manager) ClearFinished(ctx context.Context, downloadID int64) {
	var d domain.Download
	if err := database.Get(ctx, m.Store.Read(), "downloads", downloadID, &d); err != nil || d.ReleaseID == nil || d.ExternalID == "" {
		m.clearDone(ctx, downloadID)
		return
	}
	var dr domain.Downloader
	if err := database.Get(ctx, m.Store.Read(), "downloaders", d.DownloaderID, &dr); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			m.clearDone(ctx, downloadID)
		}
		return
	}
	if dr.Protocol != "usenet" || !dr.ClearFinished || readOnlyFor(dr) || dr.Removed {
		m.clearDone(ctx, downloadID)
		return
	}
	e, err := m.engine(ctx, dr)
	var folder string
	if err == nil {
		if j, gerr := e.Get(ctx, d.ExternalID); gerr == nil {
			folder = j.ContentPath
		}
		err = e.Remove(ctx, d.ExternalID, true)
	}
	if err != nil && !errors.Is(err, ErrNotFound) {
		// A Downloader that is busy or slow to answer is tried later, not
		// a failure. A real refusal goes in History once.
		if tries := m.clearLater(ctx, d.ID, err); tries == 1 && !busy(err) {
			m.event(ctx, &d, "download_not_removed", fmt.Sprintf("Could not clear the imported download from %s: %v", dr.Name, firstLine(err.Error())))
		}
		return
	}
	m.clearDone(ctx, d.ID)
	var relName string
	var rel domain.Release
	if database.Get(ctx, m.Store.Read(), "releases", *d.ReleaseID, &rel) == nil {
		relName = rel.Name
	}
	said := "Cleared the imported download from " + dr.Name
	if root, rel := m.finishedFolder(ctx, folder, relName, dr); rel != "" {
		rerr := func() error {
			rt, err := os.OpenRoot(root)
			if err != nil {
				return err
			}
			defer rt.Close()
			return rt.RemoveAll(rel)
		}()
		if rerr != nil {
			m.event(ctx, &d, "download_not_removed", fmt.Sprintf("Cleared the imported download from %s, but its files could not be deleted: %v", dr.Name, firstLine(rerr.Error())))
			return
		}
		said = "Cleared the imported download and its files from " + dr.Name
	}
	m.event(ctx, &d, "download_removed", said)
}

func (m *Manager) finishedFolder(ctx context.Context, content, relName string, dr domain.Downloader) (root, rel string) {
	if content == "" || relName == "" {
		return "", ""
	}
	local := filepath.Clean(mapLocal(content, dr.PathMap))
	if fi, err := os.Lstat(local); err == nil && fi.Mode().IsRegular() {
		local = filepath.Dir(local)
	}
	if !sameJobName(filepath.Base(local), relName) {
		return "", ""
	}
	fi, err := os.Lstat(local)
	if err != nil || !fi.IsDir() || fi.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
		return "", ""
	}
	// As for a torrent's files: never in or around a Library folder, and
	// nothing when the Library folders cannot be read.
	if keep, err := m.libraryFolders(ctx); err != nil || nearLibrary(local, keep) {
		return "", ""
	}
	var roots []string
	if m.SavePath != "" {
		roots = append(roots, m.SavePath)
	}
	for _, pm := range dr.PathMap {
		roots = append(roots, pm.Ours)
	}
	for _, r := range roots {
		if r = filepath.Clean(r); r == "" || r == "." || r == local || !underAny(local, []string{r}) {
			continue
		}
		if p, err := filepath.Rel(r, local); err == nil && p != "." && !filepath.IsAbs(p) {
			return r, p
		}
	}
	return "", ""
}

func sameJobName(folder, relName string) bool {
	return strings.EqualFold(folder, relName) || strings.EqualFold(jobSuffix.ReplaceAllString(folder, ""), relName)
}

// busy reports whether err means the Downloader did not answer in time or
// is resting after failures, so the clear is simply tried again later.
func busy(err error) bool {
	return unreachable(err) && !errors.Is(err, ErrAuth)
}

// clearWaitFirst and clearWaitMax bound the wait between clear tries: it
// doubles from the first to the ceiling.
const (
	clearWaitFirst = time.Minute
	clearWaitMax   = time.Hour
)

func clearWaitAfter(tries int) time.Duration {
	w := clearWaitFirst
	for i := 1; i < tries && w < clearWaitMax; i++ {
		w *= 2
	}
	return min(w, clearWaitMax)
}

// retryClears tries again every clear whose wait is over.
func (m *Manager) retryClears(ctx context.Context) {
	list := m.dueClears(ctx)
	for _, d := range list {
		if ctx.Err() != nil {
			return
		}
		// A row that is downloading again belongs to a new grab: its old wait
		// to clear ends and the new download is never touched.
		if isActive(d.state) {
			m.clearDone(ctx, d.id)
			continue
		}
		m.ClearFinished(ctx, d.id)
	}
}

// ClearNow tries every waiting clear at once and returns how many still wait.
func (m *Manager) ClearNow(ctx context.Context) int {
	m.dueNow(ctx)
	m.retryClears(ctx)
	return m.waitingClears(ctx)
}

// ClearWait is an imported download still waiting to be cleared from its
// Downloader: when the next try runs, how many tries failed, and the last
// reason.
type ClearWait struct {
	NextTry time.Time `json:"next_try"`
	Tries   int       `json:"tries"`
	Reason  string    `json:"reason,omitempty"`
}
