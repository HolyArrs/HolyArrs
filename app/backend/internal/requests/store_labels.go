package requests

import (
	"context"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/accounts"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediaservers"
)

// AskedForLabels lists the asked-for movies and shows in the Library that
// became available at or after since ("" for every one), with the names of
// everyone who asked, earliest first. It is the media servers' list for
// Plex labels.
func AskedForLabels(s *database.Store) mediaservers.AskedFunc {
	return func(ctx context.Context, since string) ([]mediaservers.AskedTitle, error) {
		q := s.Read()
		rows, err := q.QueryContext(ctx, `SELECT a.title_id, p.user_id FROM asks a JOIN ask_people p ON p.ask_id = a.id
			WHERE a.state = 'available' AND a.title_id IS NOT NULL AND a.kind IN ('movie', 'show') AND COALESCE(a.available_at, '') >= ?
			GROUP BY a.title_id, p.user_id ORDER BY a.title_id, min(p.asked_at), p.user_id`, since)
		if err != nil {
			return nil, err
		}
		defer rows.Close()
		type pair struct{ title, user int64 }
		var ps []pair
		for rows.Next() {
			var p pair
			if err := rows.Scan(&p.title, &p.user); err != nil {
				return nil, err
			}
			ps = append(ps, p)
		}
		if err := rows.Err(); err != nil {
			return nil, err
		}
		if len(ps) == 0 {
			return nil, nil
		}
		names, err := accounts.UserNames(ctx, q)
		if err != nil {
			return nil, err
		}
		var out []mediaservers.AskedTitle
		for _, p := range ps {
			if len(out) == 0 || out[len(out)-1].TitleID != p.title {
				out = append(out, mediaservers.AskedTitle{TitleID: p.title})
			}
			// A deleted person's stored name carries a hidden mark: leave it out.
			if n := names[p.user]; n != "" && !strings.ContainsRune(n, 0x2063) {
				out[len(out)-1].Askers = append(out[len(out)-1].Askers, n)
			}
		}
		return out, nil
	}
}
