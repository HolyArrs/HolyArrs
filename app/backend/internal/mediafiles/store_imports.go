package mediafiles

import (
	"context"
	"strings"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// RefusedDownloads reports which of the Downloads had their newest Import
// turned down (the files were refused, whatever the Slot did next), with why
// in the importer's words ("" when the plan does not say). A refusal ends
// once a person imports a file by hand into one of the download's Slots.
func RefusedDownloads(ctx context.Context, q database.Lister, ids []int64) (map[int64]string, error) {
	type last struct {
		id    int64
		state string
	}
	newest := map[int64]last{}
	err := inChunks(ids, func(part []any, marks string) error {
		rows, err := q.QueryContext(ctx, "SELECT download_id, id, state FROM imports WHERE download_id IN ("+marks+") ORDER BY id", part...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var dl int64
			var l last
			if err := rows.Scan(&dl, &l.id, &l.state); err != nil {
				return err
			}
			newest[dl] = l
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	var refused []int64
	for _, l := range newest {
		if l.state == "rejected" {
			refused = append(refused, l.id)
		}
	}
	out := map[int64]string{}
	// Why: the last reason on the refused file the download was grabbed for
	// (a sample or a stray file has no Slot). Ended: a later by-hand import
	// (no download backs it) placed a file in a Slot the refused plan meant.
	err = inChunks(refused, func(part []any, marks string) error {
		rows, err := q.QueryContext(ctx, `SELECT r.download_id,
			COALESCE((SELECT json_extract(p.value, '$.why[#-1]') FROM json_each(r.plan) p
				WHERE json_extract(p.value, '$.method') = 'refuse' AND json_array_length(p.value, '$.target_slot_ids') > 0
				ORDER BY CAST(p.key AS INTEGER) LIMIT 1), ''),
			EXISTS (SELECT 1 FROM json_each(r.plan) rp, json_each(rp.value, '$.target_slot_ids') rs, imports i, json_each(i.plan) ip, json_each(ip.value, '$.target_slot_ids') s
				WHERE i.id > r.id AND i.mode = 'by_hand' AND i.download_id IS NULL
					AND i.state IN ('done', 'partly_done') AND s.value = rs.value)
			FROM imports r WHERE r.id IN (`+marks+")", part...)
		if err != nil {
			return err
		}
		defer rows.Close()
		for rows.Next() {
			var dl int64
			var why string
			var ended bool
			if err := rows.Scan(&dl, &why, &ended); err != nil {
				return err
			}
			if !ended {
				out[dl] = why
			}
		}
		return rows.Err()
	})
	if err != nil {
		return nil, err
	}
	return out, nil
}

// inChunks calls fn for every 500 ids, with their "?, ?" marks.
func inChunks(ids []int64, fn func(part []any, marks string) error) error {
	for len(ids) > 0 {
		n := min(len(ids), 500)
		part := make([]any, n)
		for i, id := range ids[:n] {
			part[i] = id
		}
		ids = ids[n:]
		if err := fn(part, "?"+strings.Repeat(", ?", n-1)); err != nil {
			return err
		}
	}
	return nil
}
