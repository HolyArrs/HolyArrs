package sources

import (
	"context"
	"encoding/json"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// ReleaseFacts is what the old apps' History and hooks tell of a Release.
type ReleaseFacts struct {
	Size        int64
	Protocol    string
	PublishedAt string // empty when unknown
	InfoHash    string // empty when unknown
	SourceName  string
	InfoURL     string // the release's page on its Source, http(s) only; empty when unknown
	// FromSource is false when the Release's Source is gone.
	FromSource bool
}

// ReleaseFactsOf reads the facts of these Releases, by id. It returns the
// rows read so far along with any error, so a caller that only wants what it
// can get may ignore the error.
func ReleaseFactsOf(ctx context.Context, q database.Lister, ids []int64) (map[int64]ReleaseFacts, error) {
	out := map[int64]ReleaseFacts{}
	if len(ids) == 0 {
		return out, nil
	}
	in, err := json.Marshal(ids)
	if err != nil {
		return out, err
	}
	rows, err := q.QueryContext(ctx, `SELECT r.id, r.size, r.protocol, COALESCE(r.published_at, ''), COALESCE(r.info_hash, ''),
		COALESCE(s.name, ''), s.id IS NOT NULL, COALESCE(json_extract(r.parsed, '$.info_url'), '')
		FROM releases r LEFT JOIN sources s ON s.id = r.source_id WHERE r.id IN (SELECT value FROM json_each(?))`, string(in))
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var id int64
		var f ReleaseFacts
		if err := rows.Scan(&id, &f.Size, &f.Protocol, &f.PublishedAt, &f.InfoHash, &f.SourceName, &f.FromSource, &f.InfoURL); err != nil {
			return out, err
		}
		f.InfoURL = webLink(f.InfoURL)
		out[id] = f
	}
	return out, rows.Err()
}
