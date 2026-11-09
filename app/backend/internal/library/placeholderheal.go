package library

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// serverPlaceholders are the SHA-256 sums of pictures known to be only a
// stand-in for an artist's photo, which older versions stored as if it were
// the picture: the metadata server's grey stand-in (the same bytes at every
// size) and the generic grey person outline an old source sent (1000x1000,
// the one Coldplay, Ed Sheeran and Radiohead all had). StandInReason adds
// the rules that need no list (one flat colour, a flat outline).
var serverPlaceholders = map[string]bool{
	"8a121a4e3a859172c696d72ef2dc7a559ec68d1c1347422a929e4811afc06001": true,
	"0d659e802c2d9c13e8cf4e0fab3f85b176173a649b60d9ddc03af2397fb43275": true,
}

// healedSetting records that the placeholder clean-up has run.
const healedSetting = "artwork_placeholders_healed"

// HealServerPlaceholders runs once per install: it removes every cached
// picture that is only the metadata server's stand-in and every title
// reference to one (artwork, backdrop, logo), so the real picture is
// fetched again. It returns how many files it removed.
func (s *Service) HealServerPlaceholders(ctx context.Context) (int, error) {
	if s.Artwork == nil || s.Artwork.Dir == "" {
		return 0, nil
	}
	var done string
	_ = s.Store.Read().QueryRowContext(ctx, "SELECT value FROM settings WHERE grp = 'this_app' AND key = ?", healedSetting).Scan(&done)
	if done != "" {
		return 0, nil
	}
	bad := map[string]bool{}
	es, err := os.ReadDir(s.Artwork.Dir)
	if err != nil && !os.IsNotExist(err) {
		return 0, err
	}
	for _, e := range es {
		if e.IsDir() || strings.HasPrefix(e.Name(), ".") {
			continue
		}
		// The stand-in is small; real pictures are skipped without reading.
		if in, err := e.Info(); err != nil || in.Size() > 64<<10 {
			continue
		}
		p := filepath.Join(s.Artwork.Dir, e.Name())
		b, err := os.ReadFile(p)
		if err != nil {
			continue
		}
		sum := sha256.Sum256(b)
		if serverPlaceholders[hex.EncodeToString(sum[:])] && os.Remove(p) == nil {
			_ = os.Remove(filepath.Join(s.Artwork.Dir, checkName(e.Name())))
			bad[e.Name()] = true
		}
	}
	now := database.FormatTime(s.now())
	by := ActorText(domain.System)
	err = s.Store.Write(ctx, func(tx *sql.Tx) error {
		if len(bad) > 0 {
			if err := dropPlaceholderRefs(tx, bad, now, by); err != nil {
				return err
			}
		}
		_, err := tx.Exec(`INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', ?, ?, ?, ?, ?, ?)
			ON CONFLICT (grp, key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at, updated_by = excluded.updated_by`,
			healedSetting, time.Now().UTC().Format(time.RFC3339), now, by, now, by)
		return err
	})
	return len(bad), err
}

// dropPlaceholderRefs takes the removed cache names out of every title.
func dropPlaceholderRefs(tx *sql.Tx, bad map[string]bool, now, by string) error {
	rows, err := tx.Query("SELECT id, artwork, backdrop, logo FROM titles WHERE artwork LIKE '%.%' OR backdrop LIKE '/api/v1/artwork/%' OR logo LIKE '/api/v1/artwork/%'")
	if err != nil {
		return err
	}
	type fix struct {
		id              int64
		art, wide, logo string
	}
	var fixes []fix
	isBad := func(link string) bool { return bad[strings.TrimPrefix(link, "/api/v1/artwork/")] }
	for rows.Next() {
		var f fix
		var art sql.NullString
		if err := rows.Scan(&f.id, &art, &f.wide, &f.logo); err != nil {
			_ = rows.Close()
			return err
		}
		changed := false
		var list []string
		if art.Valid && json.Unmarshal([]byte(art.String), &list) == nil {
			keep := []string{}
			for _, a := range list {
				_, name, _ := strings.Cut(a, ":")
				if bad[name] || bad[a] {
					changed = true
					continue
				}
				keep = append(keep, a)
			}
			b, _ := json.Marshal(keep)
			f.art = string(b)
		} else {
			f.art = art.String
		}
		if isBad(f.wide) {
			f.wide, changed = "", true
		}
		if isBad(f.logo) {
			f.logo, changed = "", true
		}
		if changed {
			fixes = append(fixes, f)
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	for _, f := range fixes {
		if _, err := tx.Exec("UPDATE titles SET artwork = ?, backdrop = ?, logo = ?, updated_at = ?, updated_by = ? WHERE id = ?",
			f.art, f.wide, f.logo, now, by, f.id); err != nil {
			return err
		}
	}
	return nil
}
