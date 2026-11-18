package mediaservers

import (
	"context"
	"database/sql"
	"strings"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// The owner's Plex label settings, in the "links" settings group.
const (
	labelsKey      = "plex_labels"       // "on"; anything else is off
	labelTextKey   = "plex_label"        // the label; "holyarrs" when unset
	labelAskersKey = "plex_label_askers" // "on" adds each asker's name
	labelSinceKey  = "plex_labels_since" // when labels were last turned on
)

// DefaultLabel is the label put on asked-for titles unless the owner picks
// another.
const DefaultLabel = "holyarrs"

// LabelSettings is the owner's "Label requested titles" choice.
type LabelSettings struct {
	Enabled bool   `json:"enabled"`
	Label   string `json:"label"`
	Askers  bool   `json:"askers"`
	// Since is when labels were turned on: the scheduled pass labels titles
	// that became available from then on.
	Since string `json:"-"`
}

// ReadLabelSettings reads the owner's Plex label settings.
func ReadLabelSettings(ctx context.Context, q database.Querier) LabelSettings {
	s := LabelSettings{
		Enabled: database.SettingOr(ctx, q, watchGroup, labelsKey, "") == "on",
		Label:   database.SettingOr(ctx, q, watchGroup, labelTextKey, DefaultLabel),
		Askers:  database.SettingOr(ctx, q, watchGroup, labelAskersKey, "") == "on",
		Since:   database.SettingOr(ctx, q, watchGroup, labelSinceKey, ""),
	}
	if cleanLabel(s.Label) == "" {
		s.Label = DefaultLabel
	}
	return s
}

// SaveLabelSettings keeps the owner's Plex label settings; turning them on
// starts the "since" clock, so only titles that become available from now
// on are labelled on schedule.
func SaveLabelSettings(ctx context.Context, s *database.Store, in LabelSettings, by domain.Actor) error {
	return s.Write(ctx, func(tx *sql.Tx) error {
		now := time.Now().UTC()
		was := database.SettingOr(ctx, tx, watchGroup, labelsKey, "") == "on"
		onOff := func(b bool) string {
			if b {
				return "on"
			}
			return "off"
		}
		if err := database.PutSetting(tx, watchGroup, labelsKey, onOff(in.Enabled), by, now); err != nil {
			return err
		}
		if err := database.PutSetting(tx, watchGroup, labelTextKey, in.Label, by, now); err != nil {
			return err
		}
		if err := database.PutSetting(tx, watchGroup, labelAskersKey, onOff(in.Askers), by, now); err != nil {
			return err
		}
		if in.Enabled && !was {
			return database.PutSetting(tx, watchGroup, labelSinceKey, database.FormatTime(now), by, now)
		}
		return nil
	})
}

// labelsDone is each title's labels already put on one server, lower case.
func labelsDone(ctx context.Context, q database.Lister, serverID int64) (map[int64]map[string]bool, error) {
	rows, err := q.QueryContext(ctx, "SELECT title_id, label FROM media_labels WHERE media_server_id = ?", serverID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]map[string]bool{}
	for rows.Next() {
		var id int64
		var lb string
		if err := rows.Scan(&id, &lb); err != nil {
			return nil, err
		}
		if out[id] == nil {
			out[id] = map[string]bool{}
		}
		out[id][strings.ToLower(lb)] = true
	}
	return out, rows.Err()
}

// markLabels keeps that labels were put on a title on one server.
func markLabels(ctx context.Context, s *database.Store, serverID, titleID int64, labels []string, at time.Time) error {
	return s.Write(context.WithoutCancel(ctx), func(tx *sql.Tx) error {
		for _, lb := range labels {
			if _, err := tx.Exec("INSERT OR IGNORE INTO media_labels(media_server_id, title_id, label, added_at) VALUES (?, ?, ?, ?)",
				serverID, titleID, strings.ToLower(lb), database.FormatTime(at)); err != nil {
				return err
			}
		}
		return nil
	})
}
