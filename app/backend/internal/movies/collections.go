package movies

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

var (
	ErrCollectionNotFound      = errors.New("movie collection not found")
	ErrCollectionSelection     = errors.New("choose at least one owned movie from this collection")
	ErrCollectionReviewChanged = errors.New("the collection changed; review it again before removing movies")
)

type MovieCollection struct {
	ID          int64  `json:"id"`
	SourceID    string `json:"source_id"`
	Name        string `json:"name"`
	OwnedTitles int    `json:"owned_titles"`
	// TitleIDs lists the owned movies (list answer only), so the Library can
	// tag series without asking for every collection.
	TitleIDs []int64 `json:"title_ids,omitempty"`
}

type CollectionFile struct {
	ID        int64  `json:"id"`
	VersionID int64  `json:"version_id"`
	Path      string `json:"path"`
	Origin    string `json:"origin"`
	State     string `json:"state"`
	Protected bool   `json:"protected"`
}

type CollectionTitle struct {
	ID         int64            `json:"id"`
	Name       string           `json:"name"`
	Year       int              `json:"year,omitempty"`
	State      string           `json:"state"`
	VersionIDs []int64          `json:"version_ids"`
	Files      []CollectionFile `json:"files"`
	Protected  bool             `json:"protected"`
}

type CollectionDetail struct {
	MovieCollection
	Titles []CollectionTitle `json:"titles"`
}

type CollectionReviewTitle struct {
	CollectionTitle
	Selected bool   `json:"selected"`
	Skip     string `json:"skip,omitempty"`
}

type CollectionRemoveResult struct {
	TitleID int64               `json:"title_id"`
	Name    string              `json:"name"`
	Status  string              `json:"status"`
	Error   string              `json:"error,omitempty"`
	Plan    *library.RemovePlan `json:"plan,omitempty"`
}

type CollectionReview struct {
	Collection  MovieCollection          `json:"collection"`
	Titles      []CollectionReviewTitle  `json:"titles"`
	Choice      library.RemoveChoice     `json:"choice"`
	ReviewToken string                   `json:"review_token"`
	Applied     bool                     `json:"applied"`
	Results     []CollectionRemoveResult `json:"results,omitempty"`
}

// ListCollections returns provider collections with at least one currently
// owned movie. Provider membership is persisted separately; this read never
// invents membership from names or folders.
func ListCollections(ctx context.Context, db library.Reader) ([]MovieCollection, error) {
	rows, err := db.QueryContext(ctx, `SELECT c.id, c.source_id, c.name, count(t.id), group_concat(t.id)
		FROM movie_collections c
		JOIN movie_collection_titles ct ON ct.collection_id = c.id
		JOIN titles t ON t.id = ct.title_id AND t.kind = 'movie' AND t.state != 'removed'
		GROUP BY c.id, c.source_id, c.name ORDER BY lower(c.name), c.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []MovieCollection{}
	for rows.Next() {
		var c MovieCollection
		var ids string
		if err := rows.Scan(&c.ID, &c.SourceID, &c.Name, &c.OwnedTitles, &ids); err != nil {
			return nil, err
		}
		for _, s := range strings.Split(ids, ",") {
			if id, err := strconv.ParseInt(s, 10, 64); err == nil {
				c.TitleIDs = append(c.TitleIDs, id)
			}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

// CollectionDetails returns the exact currently owned movies and their
// Versions and Files. It does not read or modify media on disk.
func CollectionDetails(ctx context.Context, db library.Reader, id int64) (CollectionDetail, error) {
	var out CollectionDetail
	err := db.QueryRowContext(ctx, "SELECT id, source_id, name FROM movie_collections WHERE id = ?", id).
		Scan(&out.ID, &out.SourceID, &out.Name)
	if errors.Is(err, sql.ErrNoRows) {
		return out, ErrCollectionNotFound
	}
	if err != nil {
		return out, err
	}
	rows, err := db.QueryContext(ctx, `SELECT t.id, t.name, COALESCE(t.year, 0), t.state
		FROM movie_collection_titles ct JOIN titles t ON t.id = ct.title_id
		WHERE ct.collection_id = ? AND t.kind = 'movie' AND t.state != 'removed'
		ORDER BY lower(t.name), t.year, t.id`, id)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var t CollectionTitle
		if err := rows.Scan(&t.ID, &t.Name, &t.Year, &t.State); err != nil {
			return out, err
		}
		t.VersionIDs = []int64{}
		t.Files = []CollectionFile{}
		out.Titles = append(out.Titles, t)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	out.OwnedTitles = len(out.Titles)
	byID := make(map[int64]*CollectionTitle, len(out.Titles))
	for i := range out.Titles {
		byID[out.Titles[i].ID] = &out.Titles[i]
	}
	vs, err := db.QueryContext(ctx, `SELECT v.title_id, v.id FROM versions v
		JOIN movie_collection_titles ct ON ct.title_id = v.title_id
		JOIN titles t ON t.id = v.title_id
		WHERE ct.collection_id = ? AND t.kind = 'movie' AND t.state != 'removed'
		ORDER BY v.title_id, v.id`, id)
	if err != nil {
		return out, err
	}
	for vs.Next() {
		var titleID, versionID int64
		if err := vs.Scan(&titleID, &versionID); err != nil {
			vs.Close()
			return out, err
		}
		if t := byID[titleID]; t != nil {
			t.VersionIDs = append(t.VersionIDs, versionID)
		}
	}
	if err := vs.Close(); err != nil {
		return out, err
	}
	fs, err := db.QueryContext(ctx, `SELECT v.title_id, f.id, f.version_id, f.relative_path, f.origin, f.state, f.protected
		FROM files f JOIN versions v ON v.id = f.version_id
		JOIN movie_collection_titles ct ON ct.title_id = v.title_id
		JOIN titles t ON t.id = v.title_id
		WHERE ct.collection_id = ? AND t.kind = 'movie' AND t.state != 'removed'
		ORDER BY v.title_id, f.id`, id)
	if err != nil {
		return out, err
	}
	for fs.Next() {
		var titleID int64
		var f CollectionFile
		if err := fs.Scan(&titleID, &f.ID, &f.VersionID, &f.Path, &f.Origin, &f.State, &f.Protected); err != nil {
			fs.Close()
			return out, err
		}
		if t := byID[titleID]; t != nil {
			t.Files = append(t.Files, f)
			t.Protected = t.Protected || (f.Protected && (f.State == "present" || f.State == "missing"))
		}
	}
	if err := fs.Close(); err != nil {
		return out, err
	}
	return out, nil
}

type collectionToken struct {
	CollectionID int64                 `json:"collection_id"`
	SourceID     string                `json:"source_id"`
	Members      []CollectionTitle     `json:"members"`
	Selected     []int64               `json:"selected"`
	Choice       library.RemoveChoice  `json:"choice"`
	Plans        []collectionPlanToken `json:"plans"`
}

type collectionPlanToken struct {
	TitleID          int64                     `json:"title_id"`
	Plan             library.RemovePlan        `json:"plan"`
	ItemIDs          []int64                   `json:"item_ids"`
	HeldRoots        []string                  `json:"held_roots"`
	KeptRoots        []string                  `json:"kept_roots"`
	ProtectedRoots   []string                  `json:"protected_roots"`
	PendingDownloads []collectionDownloadToken `json:"pending_downloads"`
}

type collectionDownloadToken struct {
	ID      int64                   `json:"id"`
	State   string                  `json:"state"`
	Imports []collectionImportToken `json:"imports"`
	Jobs    []collectionJobToken    `json:"jobs"`
}

type collectionImportToken struct {
	ID    int64  `json:"id"`
	State string `json:"state"`
}

type collectionJobToken struct {
	ID    int64  `json:"id"`
	State string `json:"state"`
}

func removeRoots(files []library.RemoveFile) []string {
	out := make([]string, 0, len(files))
	for _, f := range files {
		out = append(out, f.Root())
	}
	return out
}

func pendingCollectionDownloads(ctx context.Context, db library.Reader, titleID int64) ([]collectionDownloadToken, error) {
	rows, err := db.QueryContext(ctx, `SELECT d.id, d.state FROM downloads d
		WHERE (d.state IN ('sent', 'queued', 'downloading', 'stalled', 'paused')
			OR d.state IN ('finished', 'seeding')
				AND NOT EXISTS (SELECT 1 FROM imports im WHERE im.download_id = d.id AND im.state IN ('done', 'rejected')))
		AND EXISTS (SELECT 1 FROM json_each(d.slot_ids) j JOIN slots s ON s.id = j.value JOIN items i ON i.id = s.item_id WHERE i.title_id = ?)
		ORDER BY d.id`, titleID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []collectionDownloadToken{}
	for rows.Next() {
		var d collectionDownloadToken
		if err := rows.Scan(&d.ID, &d.State); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if err := rows.Close(); err != nil {
		return nil, err
	}
	for i := range out {
		d := &out[i]
		d.Imports = []collectionImportToken{}
		imports, err := db.QueryContext(ctx, "SELECT id, state FROM imports WHERE download_id = ? ORDER BY id", d.ID)
		if err != nil {
			return nil, err
		}
		for imports.Next() {
			var im collectionImportToken
			if err := imports.Scan(&im.ID, &im.State); err != nil {
				imports.Close()
				return nil, err
			}
			d.Imports = append(d.Imports, im)
		}
		if err := imports.Err(); err != nil {
			imports.Close()
			return nil, err
		}
		if err := imports.Close(); err != nil {
			return nil, err
		}
		d.Jobs = []collectionJobToken{}
		jobs, err := db.QueryContext(ctx, `SELECT id, state FROM jobs WHERE kind = 'import_download' AND state IN ('queued', 'running')
			AND json_extract(subject, '$.type') = 'download' AND json_extract(subject, '$.id') = ? ORDER BY id`, d.ID)
		if err != nil {
			return nil, err
		}
		for jobs.Next() {
			var job collectionJobToken
			if err := jobs.Scan(&job.ID, &job.State); err != nil {
				jobs.Close()
				return nil, err
			}
			d.Jobs = append(d.Jobs, job)
		}
		if err := jobs.Err(); err != nil {
			jobs.Close()
			return nil, err
		}
		if err := jobs.Close(); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func collectionReviewToken(d CollectionDetail, selected []int64, choice library.RemoveChoice, plans []collectionPlanToken) string {
	selected = append([]int64(nil), selected...)
	sort.Slice(selected, func(i, j int) bool { return selected[i] < selected[j] })
	plans = append([]collectionPlanToken(nil), plans...)
	sort.Slice(plans, func(i, j int) bool { return plans[i].TitleID < plans[j].TitleID })
	b, _ := json.Marshal(collectionToken{CollectionID: d.ID, SourceID: d.SourceID, Members: d.Titles, Selected: selected, Choice: choice, Plans: plans})
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}

func selectedCollectionTitles(d CollectionDetail, selected []int64) ([]int64, error) {
	if len(selected) > 5000 {
		return nil, ErrCollectionSelection
	}
	owned := make(map[int64]bool, len(d.Titles))
	for _, t := range d.Titles {
		owned[t.ID] = true
	}
	if selected == nil {
		if len(d.Titles) > 5000 {
			return nil, ErrCollectionSelection
		}
		for _, t := range d.Titles {
			selected = append(selected, t.ID)
		}
	}
	seen := map[int64]bool{}
	out := make([]int64, 0, len(selected))
	for _, id := range selected {
		if id <= 0 || !owned[id] || seen[id] {
			return nil, ErrCollectionSelection
		}
		seen[id] = true
		out = append(out, id)
	}
	if len(out) == 0 {
		return nil, ErrCollectionSelection
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out, nil
}

// PreviewCollectionRemoval returns a side-effect-free, deterministic review.
// Keeping files is the zero-value/default choice.
func (s *Service) PreviewCollectionRemoval(ctx context.Context, collectionID int64, selected []int64, choice library.RemoveChoice) (CollectionReview, error) {
	unlock := s.Store.LockFiles()
	defer unlock()
	return s.previewCollectionRemovalLocked(ctx, collectionID, selected, choice)
}

func (s *Service) previewCollectionRemovalLocked(ctx context.Context, collectionID int64, selected []int64, choice library.RemoveChoice) (CollectionReview, error) {
	d, err := CollectionDetails(ctx, s.Store.Read(), collectionID)
	if err != nil {
		return CollectionReview{}, err
	}
	selected, err = selectedCollectionTitles(d, selected)
	if err != nil {
		return CollectionReview{}, err
	}
	wanted := make(map[int64]bool, len(selected))
	for _, id := range selected {
		wanted[id] = true
	}
	r := CollectionReview{Collection: d.MovieCollection, Choice: choice, Results: []CollectionRemoveResult{}}
	plans := make([]collectionPlanToken, 0, len(selected))
	for _, t := range d.Titles {
		rt := CollectionReviewTitle{CollectionTitle: t, Selected: wanted[t.ID]}
		if rt.Selected {
			p, err := s.RemoveLocked(ctx, t.ID, choice, false, domain.System)
			if err != nil {
				return CollectionReview{}, err
			}
			pendingDownloads, err := pendingCollectionDownloads(ctx, s.Store.Read(), t.ID)
			if err != nil {
				return CollectionReview{}, err
			}
			plans = append(plans, collectionPlanToken{TitleID: t.ID, Plan: p, ItemIDs: append([]int64(nil), p.ItemIDs()...),
				HeldRoots: removeRoots(p.Held), KeptRoots: removeRoots(p.Kept), ProtectedRoots: removeRoots(p.Protected), PendingDownloads: pendingDownloads})
			if p.Skipped {
				rt.Skip = "a protected copy would be retained; this whole title will stay in the Library"
			}
		}
		r.Titles = append(r.Titles, rt)
	}
	r.ReviewToken = collectionReviewToken(d, selected, choice, plans)
	return r, nil
}

// RemoveCollection confirms a fresh review and composes the existing safe
// per-title removal path. A failure is kept beside its title so the remaining
// selection can be reviewed and retried without repeating successful work.
func (s *Service) RemoveCollection(ctx context.Context, collectionID int64, selected []int64, choice library.RemoveChoice, reviewToken string, by domain.Actor) (CollectionReview, error) {
	unlock := s.Store.LockFiles()
	defer unlock()
	r, err := s.previewCollectionRemovalLocked(ctx, collectionID, selected, choice)
	if err != nil {
		if reviewToken != "" && errors.Is(err, ErrCollectionSelection) {
			return r, ErrCollectionReviewChanged
		}
		return r, err
	}
	if reviewToken == "" || !strings.EqualFold(reviewToken, r.ReviewToken) {
		return r, ErrCollectionReviewChanged
	}
	for _, t := range r.Titles {
		if !t.Selected {
			continue
		}
		if t.Skip != "" {
			r.Results = append(r.Results, CollectionRemoveResult{TitleID: t.ID, Name: t.Name, Status: "skipped", Error: t.Skip})
			continue
		}
		p, removeErr := s.RemoveLocked(ctx, t.ID, choice, true, by)
		result := CollectionRemoveResult{TitleID: t.ID, Name: t.Name, Plan: &p, Status: "removed"}
		if removeErr != nil {
			result.Status, result.Error = "failed", removeErr.Error()
			if errors.Is(removeErr, library.ErrProtectedCopy) {
				result.Status = "skipped"
			}
		}
		r.Results = append(r.Results, result)
	}
	r.Applied = true
	return r, nil
}
