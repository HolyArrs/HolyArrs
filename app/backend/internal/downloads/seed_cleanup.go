package downloads

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
)

// Remove after seeding: a torrent HolyArrs grabbed and imported leaves its
// Downloader once seeding is done, when the Downloader is set to do that (off
// by default). Seeding is done when the client stopped the torrent itself, or
// when the Downloader's ratio or time goal is met (qBittorrent and rTorrent).
// The download files go too only when asked and only when that is safe.

var (
	// ErrSeedTorrent means removing after seeding was asked of a Usenet Downloader.
	ErrSeedTorrent = errors.New("removing after seeding is for torrent Downloaders")
	// ErrSeedGoal means the seeding goal is out of range or the Downloader cannot report it.
	ErrSeedGoal = errors.New("a seeding goal works with qBittorrent and rTorrent: a ratio above 0 and up to 1000, or 1 minute to 1 year")
)

// SeedingChoice is what a Downloader does with a torrent once it is imported
// and seeding is done.
type SeedingChoice struct {
	Remove      bool     `json:"remove"`
	Ratio       *float64 `json:"ratio"`
	Minutes     *int     `json:"minutes"`
	DeleteFiles bool     `json:"delete_files"`
}

// seedGoalAdapters can report a torrent's ratio and seeding time reliably.
var seedGoalAdapters = map[string]bool{"qbittorrent": true, "rtorrent": true}

// applySeeding checks c and stores it on d.
func applySeeding(c SeedingChoice, d *domain.Downloader) error {
	if c.Ratio != nil && (*c.Ratio <= 0 || *c.Ratio > 1000) || c.Minutes != nil && (*c.Minutes < 1 || *c.Minutes > 525600) {
		return ErrSeedGoal
	}
	if (c.Ratio != nil || c.Minutes != nil) && !seedGoalAdapters[d.Adapter] {
		return ErrSeedGoal
	}
	if c.Remove && d.Protocol != "torrent" {
		return ErrSeedTorrent
	}
	d.AfterImport = "leave"
	if c.Remove {
		d.AfterImport = "remove"
	}
	d.SeedGoal = domain.SeedGoal{Ratio: c.Ratio, Minutes: c.Minutes, DeleteFiles: c.DeleteFiles}
	return nil
}

// seedingView is d's choice as the API shows it.
func seedingView(d domain.Downloader) SeedingChoice {
	return SeedingChoice{Remove: d.AfterImport == "remove", Ratio: d.SeedGoal.Ratio, Minutes: d.SeedGoal.Minutes, DeleteFiles: d.SeedGoal.DeleteFiles}
}

// tidy runs the slow checks beside the Downloader health check: remove after
// seeding, and the dead or crawling torrent rule.
func (m *Manager) tidy(ctx context.Context) {
	if m.Store == nil {
		return
	}
	m.cleanSeeded(ctx)
	m.deadRule(ctx)
}

// cleanSeeded removes every candidate whose seeding is done. Candidates are
// read from their Downloader by hash, never from a background list.
func (m *Manager) cleanSeeded(ctx context.Context) {
	found, err := seedCandidates(ctx, m.Store.Read())
	if err != nil || len(found) == 0 {
		return
	}
	// Only after a done import, with nothing placing or waiting for a person.
	var list []domain.Download
	for _, d := range found {
		if ok, err := mediafiles.ImportSettled(ctx, m.Store.Read(), d.ID); err == nil && ok {
			list = append(list, d)
		}
	}
	for did, dls := range byDownloader(list) {
		if ctx.Err() != nil {
			return
		}
		var dr domain.Downloader
		if database.Get(ctx, m.Store.Read(), "downloaders", did, &dr) != nil || readOnlyFor(dr) {
			continue
		}
		e, err := m.engine(ctx, dr)
		if err != nil {
			continue
		}
		jobs, err := readByHash(ctx, e, dr, dls)
		if err != nil {
			continue
		}
		for _, d := range dls {
			j, ok := jobs[strings.ToLower(d.ExternalID)]
			if !ok || packStillOpen(d) {
				continue
			}
			if why := seedingDone(dr, j); why != "" {
				m.removeSeeded(ctx, dr, e, d, j, why)
			}
		}
	}
}

func byDownloader(list []domain.Download) map[int64][]domain.Download {
	out := map[int64][]domain.Download{}
	for _, d := range list {
		out[d.DownloaderID] = append(out[d.DownloaderID], d)
	}
	return out
}

// readByHash reads the given downloads from their Downloader. rTorrent reads
// them by hash in one call; the others list HolyArrs's categories.
func readByHash(ctx context.Context, e Engine, dr domain.Downloader, dls []domain.Download) (map[string]Job, error) {
	ids := make([]string, 0, len(dls))
	want := map[string]bool{}
	for _, d := range dls {
		id := strings.ToLower(d.ExternalID)
		ids, want[id] = append(ids, id), true
	}
	var jobs []Job
	err := ErrNotSupported
	if al, ok := e.(ActiveLister); ok {
		jobs, err = al.ListActive(ctx, ids)
	}
	if errors.Is(err, ErrNotSupported) {
		jobs, err = listCategories(ctx, e, dr.Categories())
	}
	if err != nil {
		return nil, err
	}
	out := map[string]Job{}
	for _, j := range jobs {
		if id := strings.ToLower(j.ID); want[id] {
			out[id] = j
		}
	}
	return out, nil
}

// packStillOpen reports a season pack fetched a few episodes at a time that
// still has files switched off: more of it may be fetched later.
func packStillOpen(d domain.Download) bool {
	if d.PackFiles == nil {
		return false
	}
	for _, f := range d.FilesSeen {
		if f.Priority == 0 {
			return true
		}
	}
	return false
}

// seedingDone says why seeding of j is done, or "" while it is not. An errored
// torrent is never done.
func seedingDone(dr domain.Downloader, j Job) string {
	if j.State != StateFinished || j.Progress < 1 || clientError(j.Detail) {
		return ""
	}
	if clientStopped(dr.Adapter, j) {
		return dr.Name + " stopped seeding it"
	}
	if !seedGoalAdapters[dr.Adapter] {
		return ""
	}
	g := dr.SeedGoal
	// A ratio or time of 0 is unknown and never meets a goal.
	if g.Ratio != nil && *g.Ratio > 0 && j.Ratio > 0 && j.Ratio >= *g.Ratio {
		return fmt.Sprintf("it reached the ratio goal of %s", trimFloat(*g.Ratio))
	}
	if g.Minutes != nil && *g.Minutes > 0 && j.SeedingTime > 0 && j.SeedingTime >= int64(*g.Minutes)*60 {
		return fmt.Sprintf("it seeded for the %d minute goal", *g.Minutes)
	}
	return ""
}

func trimFloat(f float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.3f", f), "0"), ".")
}

// clientStopped reports a finished torrent its client stopped or paused by
// itself, with no error.
func clientStopped(adapter string, j Job) bool {
	if j.State != StateFinished || j.Progress < 1 || clientError(j.Detail) {
		return false
	}
	switch adapter {
	case "qbittorrent":
		return j.Detail == "pausedUP" || j.Detail == "stoppedUP"
	case "transmission":
		return j.Detail == "0"
	case "deluge":
		return strings.EqualFold(j.Detail, "paused")
	case "rtorrent", "aria2":
		return j.Detail == "complete"
	}
	return false
}

func clientError(detail string) bool {
	return strings.Contains(strings.ToLower(detail), "error") || detail == "missingFiles" || detail == "unknown"
}

// removeSeeded ends d as cleaned and asks the Downloader to let go of it. A
// client that does not answer is asked again on a later sync.
func (m *Manager) removeSeeded(ctx context.Context, dr domain.Downloader, e Engine, d domain.Download, j Job, why string) {
	deleteFiles, kept := dr.SeedGoal.DeleteFiles, ""
	if deleteFiles {
		if kept = m.keepFilesWhy(ctx, e, dr, j); kept != "" {
			deleteFiles = false
		}
	}
	reason := "removed after seeding: " + why
	next := d
	next.State, next.StateReason, next.Speed, next.UpSpeed, next.Seeds, next.Peers, next.ETA = "cleaned", &reason, 0, 0, 0, 0, nil
	var done bool
	if err := m.Store.Write(ctx, func(tx *sql.Tx) error {
		var err error
		done, err = markSeeded(tx, &next, deleteFiles)
		return err
	}); err != nil || !done {
		return
	}
	m.publish("download", next)
	if err := e.Remove(ctx, d.ExternalID, deleteFiles); err != nil && !errors.Is(err, ErrNotFound) {
		m.event(ctx, &next, "download_not_removed", fmt.Sprintf("Could not remove the seeded download from %s: %v; HolyArrs tries again on the next sync", dr.Name, firstLine(err.Error())))
		return
	}
	m.removalDone(ctx, d.ID)
	said := "Removed from " + dr.Name + " after seeding (" + why + ")"
	switch {
	case deleteFiles:
		said = "Removed from " + dr.Name + " with its files after seeding (" + why + ")"
	case kept != "":
		said += "; its files were kept: " + kept
	}
	m.event(ctx, &next, "download_removed", said)
}

// keepFilesWhy says why a seeded torrent's files must stay, or "" when they
// may go: never when a Library folder is in or around them, when they are a
// whole save folder, or when another torrent in the client uses them.
func (m *Manager) keepFilesWhy(ctx context.Context, e Engine, dr domain.Downloader, j Job) string {
	content := contentOf(j)
	if content == "" {
		return dr.Name + " did not say where they are"
	}
	local := filepath.Clean(mapLocal(content, dr.PathMap))
	keep, err := m.libraryFolders(ctx)
	if err != nil {
		return "HolyArrs could not read its Library folders"
	}
	for _, k := range keep {
		if k = strings.TrimSpace(k); k != "" && (underAny(local, []string{k}) || underAny(filepath.Clean(k), []string{local})) {
			return local + " is a Library folder or is in or around one"
		}
	}
	roots := []string{m.SavePath}
	for _, pm := range dr.PathMap {
		roots = append(roots, pm.Ours)
	}
	for _, r := range roots {
		if r = strings.TrimSpace(r); r != "" && filepath.Clean(r) == local {
			return local + " is a whole save folder"
		}
	}
	all, err := listEverything(ctx, e, dr)
	if err != nil {
		return "HolyArrs could not check whether another torrent uses them"
	}
	sep := theirSep(content)
	for _, o := range all {
		oc := contentOf(o)
		if oc == "" || strings.EqualFold(o.ID, j.ID) {
			continue
		}
		if oc == content || strings.HasPrefix(oc, content+sep) || strings.HasPrefix(content, oc+sep) {
			return "another torrent in " + dr.Name + " uses " + content
		}
	}
	return ""
}

// contentOf is where a job's data is, as the client sees it.
func contentOf(j Job) string {
	if j.ContentPath != "" {
		return j.ContentPath
	}
	if j.SavePath != "" && j.Name != "" {
		if theirSep(j.SavePath) == `\` {
			return strings.TrimRight(j.SavePath, `\`) + `\` + j.Name
		}
		return path.Join(j.SavePath, j.Name)
	}
	return ""
}

// listEverything lists every torrent the client holds, as far as it can say,
// so a twin in another category is seen.
func listEverything(ctx context.Context, e Engine, dr domain.Downloader) ([]Job, error) {
	if al, ok := e.(AllLister); ok {
		jobs, err := al.ListAll(ctx)
		if !errors.Is(err, ErrNotSupported) {
			return jobs, err
		}
	}
	all, err := e.List(ctx, "")
	if err != nil {
		return nil, err
	}
	ours, err := listCategories(ctx, e, dr.Categories())
	if err != nil {
		return nil, err
	}
	return append(all, ours...), nil
}
