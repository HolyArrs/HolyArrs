package wikidata

import (
	"context"
	"encoding/json"
	"net/url"
	"strings"
)

// CheckRevisions compares the held entities' lastrevid with Wikidata, 50
// at a time, and makes the answers of changed entities due (they refetch on
// their next request). A first sighting only records the revision.
func (s *Service) CheckRevisions(ctx context.Context) (int, error) {
	if _, err := s.db.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS revs (qid TEXT PRIMARY KEY, rev INTEGER NOT NULL) WITHOUT ROWID`); err != nil {
		return 0, err
	}
	rows, err := s.db.QueryContext(ctx, `SELECT k, body FROM answers WHERE body IS NOT NULL`)
	if err != nil {
		return 0, err
	}
	keys := map[string][]string{} // qid -> answer keys
	for rows.Next() {
		var k string
		var body []byte
		if rows.Scan(&k, &body) != nil {
			continue
		}
		var e struct {
			QID string `json:"qid"`
		}
		if json.Unmarshal(body, &e) == nil && strings.HasPrefix(e.QID, "Q") {
			keys[e.QID] = append(keys[e.QID], k)
		}
	}
	rows.Close()
	qids := make([]string, 0, len(keys))
	for q := range keys {
		qids = append(qids, q)
	}
	changed := 0
	for start := 0; start < len(qids); start += 50 {
		part := qids[start:min(len(qids), start+50)]
		var got struct {
			Entities map[string]struct {
				LastRevID int64 `json:"lastrevid"`
			} `json:"entities"`
		}
		if err := s.api(ctx, url.Values{"action": {"wbgetentities"}, "ids": {strings.Join(part, "|")}, "props": {"info"}}, &got); err != nil {
			return changed, err
		}
		for _, qid := range part {
			ent, ok := got.Entities[qid]
			if !ok || ent.LastRevID == 0 {
				continue
			}
			var old int64
			known := s.db.QueryRowContext(ctx, `SELECT rev FROM revs WHERE qid=?`, qid).Scan(&old) == nil
			if known && old != ent.LastRevID {
				for _, k := range keys[qid] {
					if _, err := s.db.ExecContext(ctx, `UPDATE answers SET fetched_at=0 WHERE k=?`, k); err == nil {
						changed++
					}
				}
			}
			if !known || old != ent.LastRevID {
				_, _ = s.db.ExecContext(ctx, `INSERT OR REPLACE INTO revs VALUES (?, ?)`, qid, ent.LastRevID)
			}
		}
	}
	return changed, nil
}
