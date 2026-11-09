package downloads

import (
	"context"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// QueueRow is one Download that is still in flight, or finished and not yet
// imported, with the names of its Release, Source and Downloader.
type QueueRow struct {
	ID             int64
	ExternalID     string
	Progress       float64
	ETA            *int64
	State          string
	StateReason    string
	SavePath       string
	SlotIDs        string // JSON list
	CreatedAt      string
	ReleaseName    string
	ReleaseSize    int64
	SourceName     string
	DownloaderName string
	Protocol       string
	FilesSeen      string // JSON list
	PathMap        string // JSON list
}

// QueueRows lists the Downloads that are in flight, or finished and still
// held by a Slot, oldest first.
func QueueRows(ctx context.Context, q database.Lister) ([]QueueRow, error) {
	rows, err := q.QueryContext(ctx, `SELECT d.id, d.external_id, d.progress, d.eta, d.state, COALESCE(d.state_reason, ''), d.save_path, d.slot_ids, d.created_at,
		COALESCE(r.name, ''), COALESCE(r.size, 0), COALESCE(src.name, ''), COALESCE(dl.name, ''), COALESCE(dl.protocol, ''), d.files_seen, COALESCE(dl.path_map, '[]')
		FROM downloads d LEFT JOIN releases r ON r.id = d.release_id LEFT JOIN sources src ON src.id = r.source_id
		LEFT JOIN downloaders dl ON dl.id = d.downloader_id
		WHERE d.state IN ('sent', 'queued', 'downloading', 'stalled', 'paused')
		OR (d.state IN ('finished', 'seeding') AND EXISTS (SELECT 1 FROM slots s WHERE s.active_download_id = d.id))
		ORDER BY d.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []QueueRow
	for rows.Next() {
		var r QueueRow
		if err := rows.Scan(&r.ID, &r.ExternalID, &r.Progress, &r.ETA, &r.State, &r.StateReason, &r.SavePath, &r.SlotIDs, &r.CreatedAt,
			&r.ReleaseName, &r.ReleaseSize, &r.SourceName, &r.DownloaderName, &r.Protocol, &r.FilesSeen, &r.PathMap); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
