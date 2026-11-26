package downloads

import (
	"context"
	"errors"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// ErrCannotList: HolyArrs can't read this kind of Downloader's categories.
var ErrCannotList = errors.New("HolyArrs cannot read the categories of this kind of Downloader")

// ClientCategories reads the category list of d's client.
func (m *Manager) ClientCategories(ctx context.Context, d domain.Downloader) ([]string, error) {
	e, err := m.engine(ctx, d)
	if err != nil {
		return nil, err
	}
	l, ok := rawEngine(e).(categoryLister)
	if !ok {
		return nil, ErrCannotList
	}
	return l.ListCategories(ctx)
}

// CategoryPicks are the client's categories that fit each kind, by kind.
func CategoryPicks(have []string) map[string]string {
	out := map[string]string{}
	for kind := range fitNames {
		if c := PickCategory(kind, have); c != "" {
			out[kind] = c
		}
	}
	return out
}

// kindFields are d's per-kind categories, by kind.
func kindFields(d *domain.Downloader) map[string]*string {
	return map[string]*string{"movie": &d.CategoryMovies, "show": &d.CategoryShows, "artist": &d.CategoryMusic}
}

// SettleCategories fits d to its client, as Sonarr does on first grab:
// when pick is set, each empty per-kind category takes the client's one
// that fits; then each category d uses that the client lacks is made there,
// when HolyArrs can and may. It returns the picks it made. Failures are
// quiet: saving never waits on categories, and the user's own are never
// renamed or removed.
func (m *Manager) SettleCategories(ctx context.Context, d domain.Downloader, pick bool) map[string]string {
	have, err := m.ClientCategories(ctx, d)
	if err != nil {
		return nil
	}
	picked := map[string]string{}
	if pick {
		for kind, f := range kindFields(&d) {
			if *f != "" {
				continue
			}
			if c := PickCategory(kind, have); c != "" {
				*f, picked[kind] = c, c
			}
		}
	}
	if readOnlyFor(d) {
		return picked
	}
	e, err := m.engine(ctx, d)
	if err != nil {
		return picked
	}
	mk, ok := rawEngine(e).(categoryMaker)
	if !ok {
		return picked
	}
	for _, c := range usedCategories(d) {
		if c != "*" && !hasCategory(have, c) {
			_ = mk.MakeCategory(ctx, c)
		}
	}
	return picked
}

// usedCategories are the categories d sends grabs to: each per-kind one,
// and the main one while a kind still falls back to it.
func usedCategories(d domain.Downloader) []string {
	var out []string
	fallback := false
	for _, c := range []string{d.CategoryMovies, d.CategoryShows, d.CategoryMusic} {
		if c == "" {
			fallback = true
		} else if !hasCategory(out, c) {
			out = append(out, c)
		}
	}
	if fallback && d.Category != "" && !hasCategory(out, d.Category) {
		out = append(out, d.Category)
	}
	return out
}

// settleImported fits each imported Downloader to its client once per run:
// a kind the old apps left empty takes the client's fitting category, and a
// missing one is made, as a Downloader added here gets on save. Imports never
// reach the client themselves, so this is where they meet it.
func (m *Manager) settleImported(ctx context.Context) {
	if m.Store == nil {
		return
	}
	dls, err := database.Select[domain.Downloader](ctx, m.Store.Read(), "downloaders", "removed = 0 AND state != 'disabled' ORDER BY id")
	if err != nil {
		return
	}
	for _, d := range dls {
		if len(d.CategoriesImported) == 0 || m.wasSettled(d.ID) {
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, 20*time.Second)
		if _, err := m.ClientCategories(cctx, d); err == nil {
			m.markSettled(d.ID)
			picked := m.SettleCategories(cctx, d, true)
			for kind, c := range picked {
				_ = fillKindCategory(cctx, m.Store, d.ID, kind, c)
			}
			if len(picked) > 0 {
				m.Forget(d.ID)
			}
		}
		cancel()
	}
}

func (m *Manager) wasSettled(id int64) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.settled[id]
}

func (m *Manager) markSettled(id int64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.settled == nil {
		m.settled = map[int64]bool{}
	}
	m.settled[id] = true
}
