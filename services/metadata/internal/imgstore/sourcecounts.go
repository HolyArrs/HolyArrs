package imgstore

import "context"

// SourceCount is one line of the music source census: how many stored
// values of one kind came from one source, and how many of them lack a
// source or a fetch date.
type SourceCount struct {
	Kind     string `json:"kind"`
	Source   string `json:"source"`
	N        int64  `json:"n"`
	Untagged int64  `json:"untagged"`
}

// MusicSourceCounts counts the music pictures (artist photos, album
// covers, wide backgrounds, logos) per kind and source, and the live
// MusicBrainz tags kept per item kind (source "mb-live"). A value is
// untagged when it has no source or no fetch date. Titles and people are
// not counted.
func (s *Store) MusicSourceCounts(ctx context.Context) ([]SourceCount, error) {
	var out []SourceCount
	for _, q := range []string{
		`SELECT kind, source, COUNT(*), SUM(CASE WHEN source = '' OR fetched <= 0 THEN 1 ELSE 0 END)
 FROM images WHERE kind IN ('artist','album','wide','logo') GROUP BY kind, source`,
		`SELECT kind, source, COUNT(*), SUM(CASE WHEN source = '' OR fetched_at <= 0 THEN 1 ELSE 0 END)
 FROM source_ids WHERE source = 'mb-live' AND ext <> '' GROUP BY kind, source`,
	} {
		rows, err := s.db.QueryContext(ctx, q)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var c SourceCount
			if err := rows.Scan(&c.Kind, &c.Source, &c.N, &c.Untagged); err != nil {
				rows.Close()
				return nil, err
			}
			out = append(out, c)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return nil, err
		}
	}
	return out, nil
}
