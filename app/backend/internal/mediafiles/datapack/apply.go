package datapack

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

var ErrStaleApproval = errors.New("data pack preview is stale")

type Selection struct {
	PickRuleIDs []string `json:"pick_rule_ids"`
	NamingID    string   `json:"naming_id,omitempty"`
}

type RulePreview struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Occupied bool   `json:"occupied"`
}

type NamingChange struct {
	Key      string  `json:"key"`
	Current  *string `json:"current"`
	Proposed string  `json:"proposed"`
}

type NamingPreview struct {
	ID      string         `json:"id"`
	Name    string         `json:"name"`
	Changes []NamingChange `json:"changes"`
}

type ApplyPreview struct {
	PackID       string         `json:"pack_id"`
	PackDigest   string         `json:"pack_digest"`
	PickRules    []RulePreview  `json:"pick_rules"`
	Naming       *NamingPreview `json:"naming,omitempty"`
	ApprovalHash string         `json:"approval_hash"`
	selection    Selection
	pack         Pack
	baseline     approvalBaseline
}

type ApplyResult struct {
	PackID      string   `json:"pack_id"`
	PickRuleIDs []string `json:"pick_rule_ids"`
	NamingID    string   `json:"naming_id,omitempty"`
}

type baselineValue struct {
	Present bool   `json:"present"`
	Value   string `json:"value"`
}

type approvalBaseline struct {
	Version       int                      `json:"version"`
	PackDigest    string                   `json:"pack_digest"`
	PickRuleIDs   []string                 `json:"pick_rule_ids"`
	NamingID      string                   `json:"naming_id,omitempty"`
	RuleOccupancy []RulePreview            `json:"rule_occupancy"`
	NamingValues  map[string]baselineValue `json:"naming_values"`
}

type queryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func Preview(ctx context.Context, st *database.Store, packID string, selection Selection) (ApplyPreview, error) {
	return buildPreview(ctx, st.Read(), packID, selection)
}

func buildPreview(ctx context.Context, q queryer, packID string, selection Selection) (ApplyPreview, error) {
	p, canonical, err := loadPack(ctx, q, packID)
	if err != nil {
		return ApplyPreview{}, err
	}
	selection, rules, chosenNaming, err := selectItems(p, selection)
	if err != nil {
		return ApplyPreview{}, err
	}
	digest := sha256.Sum256(canonical)
	preview := ApplyPreview{PackID: p.ID, PackDigest: hex.EncodeToString(digest[:]), PickRules: make([]RulePreview, 0, len(rules)), selection: selection, pack: p}
	for _, rule := range rules {
		var id int64
		err := q.QueryRowContext(ctx, `SELECT id FROM pick_rules WHERE name = ?`, rule.Name).Scan(&id)
		occupied := err == nil
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return ApplyPreview{}, err
		}
		preview.PickRules = append(preview.PickRules, RulePreview{ID: rule.ID, Name: rule.Name, Occupied: occupied})
	}
	base := approvalBaseline{Version: 1, PackDigest: preview.PackDigest, PickRuleIDs: slices.Clone(selection.PickRuleIDs), NamingID: selection.NamingID,
		RuleOccupancy: slices.Clone(preview.PickRules), NamingValues: map[string]baselineValue{}}
	if chosenNaming != nil {
		proposed := namingValues(chosenNaming.Patterns)
		keys := sortedKeys(proposed)
		np := &NamingPreview{ID: chosenNaming.ID, Name: chosenNaming.Name, Changes: make([]NamingChange, 0, len(keys))}
		for _, key := range keys {
			var current string
			err := q.QueryRowContext(ctx, `SELECT value FROM settings WHERE grp = 'where_files_go' AND key = ?`, key).Scan(&current)
			value := baselineValue{}
			var currentPtr *string
			switch {
			case err == nil:
				value = baselineValue{Present: true, Value: current}
				copyValue := current
				currentPtr = &copyValue
			case errors.Is(err, sql.ErrNoRows):
			default:
				return ApplyPreview{}, err
			}
			base.NamingValues[key] = value
			np.Changes = append(np.Changes, NamingChange{Key: key, Current: currentPtr, Proposed: proposed[key]})
		}
		current, err := mediafiles.ReadNaming(ctx, q)
		if err != nil {
			return ApplyPreview{}, err
		}
		merged := overlayNaming(current, chosenNaming.Patterns)
		if err := merged.Validate(); err != nil {
			return ApplyPreview{}, invalidInput(fmt.Errorf("selected naming set is invalid with current settings: %w", err))
		}
		preview.Naming = np
	}
	approvalJSON, err := json.Marshal(base)
	if err != nil {
		return ApplyPreview{}, err
	}
	hash := sha256.Sum256(approvalJSON)
	preview.ApprovalHash = hex.EncodeToString(hash[:])
	preview.baseline = base
	return preview, nil
}

func Apply(ctx context.Context, st *database.Store, packID string, selection Selection, approvalHash string, by domain.Actor) (ApplyResult, error) {
	if len(approvalHash) != sha256.Size*2 {
		return ApplyResult{}, ErrStaleApproval
	}
	var result ApplyResult
	err := st.Write(ctx, func(tx *sql.Tx) error {
		preview, err := buildPreview(ctx, tx, packID, selection)
		if err != nil {
			return err
		}
		for _, rule := range preview.PickRules {
			if rule.Occupied {
				return fmt.Errorf("%w: pick rule name %q is already used", ErrConflict, rule.Name)
			}
		}
		if preview.ApprovalHash != approvalHash {
			return ErrStaleApproval
		}
		selected, rules, chosenNaming, err := selectItems(preview.pack, preview.selection)
		if err != nil {
			return err
		}
		for _, rule := range rules {
			if _, err := database.Insert(tx, "pick_rules", &mediafiles.PickRules{Name: rule.Name, Kinds: slices.Clone(rule.Kinds), Revision: 1, Preset: false, Body: rule.Rules}, by); err != nil {
				return err
			}
		}
		if chosenNaming != nil {
			current, err := mediafiles.ReadNaming(ctx, tx)
			if err != nil {
				return err
			}
			if err := overlayNaming(current, chosenNaming.Patterns).Validate(); err != nil {
				return err
			}
			actor, _ := json.Marshal(by)
			now := database.FormatTime(time.Now())
			values := namingValues(chosenNaming.Patterns)
			for _, key := range sortedKeys(values) {
				if _, err := tx.ExecContext(ctx, `INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('where_files_go', ?, ?, ?, ?, ?, ?)
					ON CONFLICT (grp, key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at, updated_by=excluded.updated_by`,
					key, values[key], now, string(actor), now, string(actor)); err != nil {
					return err
				}
			}
		}
		result = ApplyResult{PackID: preview.pack.ID, PickRuleIDs: slices.Clone(selected.PickRuleIDs), NamingID: selected.NamingID}
		return nil
	})
	if err != nil {
		return ApplyResult{}, err
	}
	return result, nil
}

func loadPack(ctx context.Context, q queryer, id string) (Pack, []byte, error) {
	if err := validID("data pack", id); err != nil {
		return Pack{}, nil, invalidInput(err)
	}
	var raw string
	err := q.QueryRowContext(ctx, `SELECT value FROM settings WHERE grp='this_app' AND key=?`, "data_pack:"+id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return Pack{}, nil, ErrNotFound
	}
	if err != nil {
		return Pack{}, nil, err
	}
	p, err := Decode([]byte(raw))
	if err != nil {
		return Pack{}, nil, fmt.Errorf("stored data pack is invalid: %w", err)
	}
	canonical, err := Encode(p)
	return p, canonical, err
}

func selectItems(p Pack, selection Selection) (Selection, []PickRule, *NamingSet, error) {
	if len(selection.PickRuleIDs) == 0 && selection.NamingID == "" {
		return Selection{}, nil, nil, invalidInput(errors.New("select at least one pick rule or naming set"))
	}
	ids := slices.Clone(selection.PickRuleIDs)
	slices.Sort(ids)
	for i, id := range ids {
		if err := validID("pick rule", id); err != nil {
			return Selection{}, nil, nil, invalidInput(err)
		}
		if i > 0 && ids[i-1] == id {
			return Selection{}, nil, nil, invalidInput(fmt.Errorf("duplicate selected pick rule id %q", id))
		}
	}
	byID := make(map[string]PickRule, len(p.PickRules))
	for _, rule := range p.PickRules {
		byID[rule.ID] = rule
	}
	rules := make([]PickRule, 0, len(ids))
	for _, id := range ids {
		rule, ok := byID[id]
		if !ok {
			return Selection{}, nil, nil, invalidInput(fmt.Errorf("pick rule id %q is not in the data pack", id))
		}
		rules = append(rules, rule)
	}
	var chosen *NamingSet
	if selection.NamingID != "" {
		if err := validID("naming set", selection.NamingID); err != nil {
			return Selection{}, nil, nil, invalidInput(err)
		}
		for i := range p.Naming {
			if p.Naming[i].ID == selection.NamingID {
				copySet := p.Naming[i]
				chosen = &copySet
				break
			}
		}
		if chosen == nil {
			return Selection{}, nil, nil, invalidInput(fmt.Errorf("naming set id %q is not in the data pack", selection.NamingID))
		}
	}
	return Selection{PickRuleIDs: ids, NamingID: selection.NamingID}, rules, chosen, nil
}

func namingValues(p NamingPatterns) map[string]string {
	return map[string]string{
		"file_pattern": p.FilePattern, "folder_pattern": p.FolderPattern, "show_folder_pattern": p.ShowFolderPattern,
		"season_folder_pattern": p.SeasonFolderPattern, "specials_folder": p.SpecialsFolder, "episode_pattern": p.EpisodePattern,
		"daily_pattern": p.DailyPattern, "anime_pattern": p.AnimePattern, "artist_folder_pattern": p.ArtistFolderPattern,
		"album_folder_pattern": p.AlbumFolderPattern, "track_pattern": p.TrackPattern, "multi_disc_track_pattern": p.MultiDiscTrackPattern,
		"disc_folder_pattern": p.DiscFolderPattern, "colon": p.Colon, "illegal": p.Illegal,
		"multi_episode_style": p.MultiEpisodeStyle, "air_date_style": p.AirDateStyle, "absolute_digits": strconv.Itoa(p.AbsoluteDigits),
	}
}

func overlayNaming(s mediafiles.NamingSettings, p NamingPatterns) mediafiles.NamingSettings {
	s.FilePattern, s.FolderPattern = p.FilePattern, p.FolderPattern
	s.ShowFolderPattern, s.SeasonFolderPattern, s.SpecialsFolder = p.ShowFolderPattern, p.SeasonFolderPattern, p.SpecialsFolder
	s.EpisodePattern, s.DailyPattern, s.AnimePattern = p.EpisodePattern, p.DailyPattern, p.AnimePattern
	s.ArtistFolderPattern, s.AlbumFolderPattern = p.ArtistFolderPattern, p.AlbumFolderPattern
	s.TrackPattern, s.MultiDiscTrackPattern, s.DiscFolderPattern = p.TrackPattern, p.MultiDiscTrackPattern, p.DiscFolderPattern
	s.Colon, s.Illegal = p.Colon, p.Illegal
	s.MultiEpisodeStyle, s.AirDateStyle, s.AbsoluteDigits = p.MultiEpisodeStyle, p.AirDateStyle, p.AbsoluteDigits
	return s
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}
