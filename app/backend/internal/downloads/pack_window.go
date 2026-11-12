package downloads

import (
	"context"
	"errors"
	"fmt"
	"path"
	"slices"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer"
)

// A show can take a season pack a few episodes at a time: the torrent is
// added stopped, only the files of the next few episodes are switched on,
// and more are switched on later (Widen). The Download keeps the pack's
// episode files in pack_files. qBittorrent only.

var (
	// ErrNoPack means no season pack is being fetched a few episodes at a time.
	ErrNoPack = errors.New("no season pack of this show is being fetched a few episodes at a time")
	// ErrPackChanged means the pack's files changed while Widen ran.
	ErrPackChanged = errors.New("the season pack changed while switching on episodes; try again")
	// ErrPackGone means the Downloader no longer has the season pack.
	ErrPackGone = errors.New("the Downloader no longer has the season pack; its episodes are searched again")
)

// packWindow is how many episodes ahead a Release is fetched when it is a
// torrent season pack for a show that takes packs a few episodes at a time,
// else 0.
func packWindow(ctx context.Context, db database.Querier, rel domain.Release, itemID int64) int {
	if rel.Protocol != "torrent" || !mediafiles.IsPack(releasename.Parse(rel.Name, releasename.Hints{Show: true})) {
		return 0
	}
	n, _ := titlePackWindow(ctx, db, itemID)
	return n
}

// packDownloaders keeps the Downloaders that can fetch part of a pack:
// qBittorrent that HolyArrs may change.
func packDownloaders(dls []domain.Downloader) []domain.Downloader {
	var out []domain.Downloader
	for _, d := range dls {
		if d.Adapter == "qbittorrent" && !readOnlyFor(d) {
			out = append(out, d)
		}
	}
	return out
}

// PackReady says whether a show with these tags can take season packs a few
// episodes at a time: a qBittorrent HolyArrs may change takes its torrents.
func (m *Manager) PackReady(ctx context.Context, titleTags []string) bool {
	all, err := database.Select[domain.Downloader](ctx, m.Store.Read(), "downloaders", "protocol = 'torrent' AND removed = 0 ORDER BY priority, id")
	if err != nil {
		return false
	}
	return len(packDownloaders(mediafiles.Downloaders(all, titleTags, func(d domain.Downloader) []string { return d.Tags }))) > 0
}

// packPending chooses the files of a season pack added stopped, once the
// Downloader lists them, and starts it. Without a file list yet it waits for
// the next poll. Any error after that removes the torrent and fails the
// Download, so the next Release is tried; it is never started then. Only a
// torrent HolyArrs added gets here, never another app's of the same hash.
func (m *Manager) packPending(ctx context.Context, dr domain.Downloader, d domain.Download) {
	var files []TorrentFile
	err := m.ControlEngine(ctx, dr, func(e Engine) error {
		return asInspector(e, func(t TorrentInspector) error {
			var err error
			files, err = t.TorrentFiles(ctx, d.ExternalID)
			return err
		})
	})
	if err != nil && !errors.Is(err, ErrNotSupported) {
		return // the Downloader did not answer: the next poll tries again
	}
	if err == nil && len(files) == 0 {
		// Not known yet. A magnet that gets no file list in the stall time fails, so
		// the next Release is tried.
		wait := m.now().Sub(lastMoved(d))
		if wait < m.settings(ctx).stallAfter {
			return
		}
		err = fmt.Errorf("the Downloader got no file list in %s", hours(wait))
	}
	if err == nil {
		err = m.choosePackFiles(ctx, dr, d, files)
	}
	if err == nil {
		return
	}
	_ = m.ControlEngine(ctx, dr, func(e Engine) error { return e.Remove(ctx, d.ExternalID, true) })
	next := d
	next.State = "failed"
	reason := "the season pack's files could not be chosen: " + err.Error()
	next.StateReason = &reason
	m.failed(ctx, dr, d, next, m.settings(ctx), domain.System)
}

// choosePackFiles matches the torrent's files to episodes, skips every file
// outside the Download's Slots, starts the torrent and saves pack_files.
func (m *Manager) choosePackFiles(ctx context.Context, dr domain.Downloader, d domain.Download, files []TorrentFile) error {
	pack, err := m.buildPack(ctx, d, files)
	if err != nil {
		return err
	}
	var on, off []int
	for i := range files {
		if slices.ContainsFunc(pack, func(f domain.PackFile) bool { return f.On && f.I == i }) {
			on = append(on, i)
		} else {
			off = append(off, i)
		}
	}
	if len(on) == 0 {
		return errors.New("it holds no file for the episodes asked for")
	}
	err = m.ControlEngine(ctx, dr, func(e Engine) error {
		c, ok := e.(Controller)
		if !ok {
			return ErrNotSupported
		}
		return asInspector(e, func(t TorrentInspector) error {
			if err := t.SetFilePriority(ctx, d.ExternalID, off, 0); err != nil {
				return err
			}
			if err := t.SetFilePriority(ctx, d.ExternalID, on, 1); err != nil {
				return err
			}
			return c.ResumeItem(ctx, d.ExternalID)
		})
	})
	if err != nil {
		return err
	}
	d.PackFiles = pack
	if err := savePackFiles(ctx, m.Store, d.ID, pack); err != nil {
		return err
	}
	m.publish("download", d)
	return nil
}

// buildPack matches each torrent file to the episodes of the window's season
// with the release name parser, in episode order. When two files hold the
// same episodes (a sample beside the video), the larger is kept. A file is on
// when it fills one of the Download's Slots.
func (m *Manager) buildPack(ctx context.Context, d domain.Download, files []TorrentFile) ([]domain.PackFile, error) {
	if len(d.SlotIDs) == 0 {
		return nil, errors.New("the download has no Slot")
	}
	eps, order, err := packEpisodes(ctx, m.Store.Read(), d.SlotIDs[0])
	if err != nil {
		return nil, err
	}
	refs := make([]mediafiles.EpisodeRef, len(eps))
	byItem := map[int64]packEp{}
	for i, e := range eps {
		refs[i] = e.ref
		byItem[e.ref.ItemID] = e
	}
	var out []domain.PackFile
	sizes := map[int]int64{}
	for i, f := range files {
		if !importer.IsVideo(path.Ext(f.Name)) {
			continue
		}
		p := releasename.Parse(path.Base(f.Name), releasename.Hints{Show: true})
		if mediafiles.IsPack(p) {
			continue
		}
		found, why := mediafiles.MatchEpisodes(p, order, refs)
		if why != "" || len(found) == 0 {
			continue
		}
		pf := domain.PackFile{I: i, Name: f.Name, Slots: []int64{}, Items: []int64{}, Eps: []int{}}
		for _, r := range found {
			pf.Items = append(pf.Items, r.ItemID)
			pf.Eps = append(pf.Eps, r.Episode)
			if s := byItem[r.ItemID].slotID; s > 0 {
				pf.Slots = append(pf.Slots, s)
				pf.On = pf.On || slices.Contains(d.SlotIDs, s)
			}
		}
		same := slices.IndexFunc(out, func(o domain.PackFile) bool { return slices.Equal(o.Items, pf.Items) })
		switch {
		case same < 0:
			out = append(out, pf)
		case f.Size > sizes[out[same].I]:
			out[same] = pf
		default:
			continue
		}
		sizes[i] = f.Size
	}
	slices.SortStableFunc(out, func(a, b domain.PackFile) int { return a.Eps[0] - b.Eps[0] })
	return out, nil
}

// Widen switches on the next n episode files of a season pack after the
// highest one on, in episode order, counting only episodes still wanted and
// kept up to date, and stopping at the end of the pack. Their Slots join the
// Download and move to downloading, and the Download goes back to
// downloading, so the poll follows it again. It never adds the torrent again.
// A torrent the Downloader no longer has clears pack_files and returns
// ErrPackGone; the episodes not fetched are still wanted and searched again.
//
// Widen calls run one at a time and read the Download afresh under that lock,
// so two at once never pick the same episodes. The database records which
// files are switched on first; if the Downloader then refuses, that record
// is put back.
func (m *Manager) Widen(ctx context.Context, downloadID int64, n int, by domain.Actor) (int, error) {
	m.packMu.Lock()
	defer m.packMu.Unlock()
	db := m.Store.Read()
	var d domain.Download
	if err := database.Get(ctx, db, "downloads", downloadID, &d); err != nil {
		return 0, err
	}
	if len(d.PackFiles) == 0 || d.ReleaseID == nil {
		return 0, ErrNoPack
	}
	var dr domain.Downloader
	if err := database.Get(ctx, db, "downloaders", d.DownloaderID, &dr); err != nil {
		return 0, err
	}
	err := m.ControlEngine(ctx, dr, func(e Engine) error { _, err := e.Get(ctx, d.ExternalID); return err })
	if errors.Is(err, ErrNotFound) {
		if err := savePackFiles(ctx, m.Store, d.ID, nil); err != nil {
			return 0, err
		}
		return 0, ErrPackGone
	}
	if err != nil {
		return 0, err
	}
	cursor := -1
	for i, f := range d.PackFiles {
		if f.On {
			cursor = i
		}
	}
	var pick []int
	for i := cursor + 1; i < len(d.PackFiles) && len(pick) < n; i++ {
		if ok, err := packSlotsWanted(ctx, db, d.PackFiles[i].Slots); err != nil {
			return 0, err
		} else if ok {
			pick = append(pick, i)
		}
	}
	if len(pick) == 0 {
		return 0, nil
	}
	idx := make([]int, len(pick))
	for k, i := range pick {
		idx[k] = d.PackFiles[i].I
	}
	was := slices.Clone(d.PackFiles)
	var slots []int64
	for _, i := range pick {
		d.PackFiles[i].On = true
		for _, s := range d.PackFiles[i].Slots {
			if !slices.Contains(d.SlotIDs, s) {
				d.SlotIDs = append(d.SlotIDs, s)
				slots = append(slots, s)
			}
		}
	}
	if err := claimPack(ctx, m.Store, d.ID, was, d.PackFiles); err != nil {
		return 0, err
	}
	err = m.ControlEngine(ctx, dr, func(e Engine) error {
		return asInspector(e, func(t TorrentInspector) error { return t.SetFilePriority(ctx, d.ExternalID, idx, 1) })
	})
	if err != nil {
		if uerr := claimPack(ctx, m.Store, d.ID, d.PackFiles, was); uerr != nil {
			err = errors.Join(err, uerr)
		}
		return 0, fmt.Errorf("switching on the next episodes in %s: %w", dr.Name, err)
	}
	if err := widenPack(ctx, m.Store, &d, slots, m.now(), by); err != nil {
		return 0, err
	}
	for _, s := range slots {
		m.publish("slot", map[string]any{"id": s, "state": mediafiles.SlotDownloading})
	}
	m.publish("download", d)
	m.Poke()
	return len(pick), nil
}

// Advance keeps each season pack of a show that is fetched a few episodes at
// a time that many episodes ahead: the episodes switched on that nobody
// watched (watched holds episode Item ids). The media server watch check
// calls it.
func (m *Manager) Advance(ctx context.Context, titleID int64, watched map[int64]bool, by domain.Actor) error {
	ds, window, err := partialPacks(ctx, m.Store.Read(), titleID, 0)
	if err != nil || window <= 0 {
		return err
	}
	var errs []error
	for _, d := range ds {
		ahead := 0
		for _, f := range d.PackFiles {
			for _, it := range f.Items {
				if f.On && !watched[it] {
					ahead++
				}
			}
		}
		if ahead < window {
			if _, err := m.Widen(ctx, d.ID, window-ahead, by); err != nil && !errors.Is(err, ErrPackGone) {
				errs = append(errs, err)
			}
		}
	}
	return errors.Join(errs...)
}

// PackNext switches on the next episodes of a show's season pack (as many as
// the show takes ahead) and returns the Download and how many were switched
// on. ErrNoPack means that season has no pack fetched a few at a time.
func (m *Manager) PackNext(ctx context.Context, titleID int64, season int, by domain.Actor) (int64, int, error) {
	ds, window, err := partialPacks(ctx, m.Store.Read(), titleID, season)
	if err != nil {
		return 0, 0, err
	}
	if len(ds) == 0 {
		return 0, 0, ErrNoPack
	}
	if window <= 0 {
		window = 2
	}
	n, err := m.Widen(ctx, ds[0].ID, window, by)
	return ds[0].ID, n, err
}
