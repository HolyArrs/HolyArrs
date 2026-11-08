// Package datapack validates and stores inert, code-free data packs.
package datapack

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

const (
	Format          = "holyarrs-data-pack"
	Version         = 1
	MaxBytes        = 256 << 10
	MaxPickRules    = 32
	MaxNamingSets   = 8
	MaxReleaseTests = 256
	maxText         = 200
	maxReleaseName  = 512
)

var (
	ErrConflict     = errors.New("data pack already exists")
	ErrInvalidInput = errors.New("invalid data pack input")
	ErrNotFound     = errors.New("data pack not found")
	ErrTooLarge     = errors.New("data pack is larger than 256 KiB")
	idRE            = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9._-]{0,62}[a-z0-9])?$`)
)

type inputError struct{ error }

func (inputError) Unwrap() error { return ErrInvalidInput }

func invalidInput(err error) error { return inputError{err} }

type Pack struct {
	Format       string        `json:"format"`
	Version      int           `json:"version"`
	ID           string        `json:"id"`
	Name         string        `json:"name"`
	PickRules    []PickRule    `json:"pick_rules,omitempty"`
	Naming       []NamingSet   `json:"naming,omitempty"`
	ReleaseTests []ReleaseTest `json:"release_tests,omitempty"`
}

type PickRule struct {
	ID    string                   `json:"id"`
	Name  string                   `json:"name"`
	Kinds []string                 `json:"kinds"`
	Rules mediafiles.PickRulesBody `json:"rules"`
}

// NamingPatterns is the naming whitelist a pack may carry. It deliberately
// excludes scan limits, rename-on-import, folder switches and tag-reading.
type NamingPatterns struct {
	FilePattern           string `json:"file_pattern"`
	FolderPattern         string `json:"folder_pattern"`
	ShowFolderPattern     string `json:"show_folder_pattern"`
	SeasonFolderPattern   string `json:"season_folder_pattern"`
	SpecialsFolder        string `json:"specials_folder"`
	EpisodePattern        string `json:"episode_pattern"`
	DailyPattern          string `json:"daily_pattern"`
	AnimePattern          string `json:"anime_pattern"`
	ArtistFolderPattern   string `json:"artist_folder_pattern"`
	AlbumFolderPattern    string `json:"album_folder_pattern"`
	TrackPattern          string `json:"track_pattern"`
	MultiDiscTrackPattern string `json:"multi_disc_track_pattern"`
	DiscFolderPattern     string `json:"disc_folder_pattern"`
	Colon                 string `json:"colon"`
	Illegal               string `json:"illegal"`
	MultiEpisodeStyle     string `json:"multi_episode_style"`
	AirDateStyle          string `json:"air_date_style"`
	AbsoluteDigits        int    `json:"absolute_digits"`
}

type NamingSet struct {
	ID       string         `json:"id"`
	Name     string         `json:"name"`
	Patterns NamingPatterns `json:"patterns"`
}

type ReleaseHints struct {
	Show  bool `json:"show"`
	Music bool `json:"music"`
}

type ExpectedFacts struct {
	Kind       string   `json:"kind"`
	Quality    string   `json:"quality"`
	Resolution int      `json:"resolution"`
	Source     string   `json:"source"`
	Codec      string   `json:"codec"`
	Flags      []string `json:"flags"`
}

type ReleaseTest struct {
	ID          string        `json:"id"`
	Name        string        `json:"name"`
	ReleaseName string        `json:"release_name"`
	Hints       ReleaseHints  `json:"hints"`
	Expected    ExpectedFacts `json:"expected"`
}

type Summary struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Status       string `json:"status"`
	PickRules    int    `json:"pick_rules"`
	NamingSets   int    `json:"naming_sets"`
	ReleaseTests int    `json:"release_tests"`
}

// Decode strictly decodes and validates one complete pack.
func Decode(raw []byte) (Pack, error) {
	if len(raw) > MaxBytes {
		return Pack{}, ErrTooLarge
	}
	if !utf8.Valid(raw) {
		return Pack{}, errors.New("data pack is not valid UTF-8")
	}
	if err := rejectDuplicateKeys(raw); err != nil {
		return Pack{}, err
	}
	var p Pack
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(&p); err != nil {
		return Pack{}, fmt.Errorf("decode data pack: %w", err)
	}
	if err := requireEOF(d); err != nil {
		return Pack{}, err
	}
	if err := p.Validate(); err != nil {
		return Pack{}, err
	}
	return p, nil
}

// Encode validates and returns deterministic compact JSON.
func Encode(p Pack) ([]byte, error) {
	if err := p.Validate(); err != nil {
		return nil, err
	}
	b, err := json.Marshal(p)
	if err != nil {
		return nil, err
	}
	if len(b) > MaxBytes {
		return nil, ErrTooLarge
	}
	return b, nil
}

func (p Pack) Validate() error {
	if p.Format != Format || p.Version != Version {
		return fmt.Errorf("data pack format/version must be %q version %d", Format, Version)
	}
	if err := validID("data pack", p.ID); err != nil {
		return err
	}
	if err := validName("data pack", p.Name); err != nil {
		return err
	}
	if len(p.PickRules) > MaxPickRules || len(p.Naming) > MaxNamingSets || len(p.ReleaseTests) > MaxReleaseTests {
		return errors.New("data pack exceeds an item limit")
	}
	if len(p.PickRules)+len(p.Naming)+len(p.ReleaseTests) == 0 {
		return errors.New("data pack is empty")
	}
	if err := validateIDs("pick rule", len(p.PickRules), func(i int) string { return p.PickRules[i].ID }); err != nil {
		return err
	}
	for i := range p.PickRules {
		if err := p.PickRules[i].validate(); err != nil {
			return fmt.Errorf("pick_rules[%d]: %w", i, err)
		}
	}
	seenRuleNames := map[string]bool{}
	for _, rule := range p.PickRules {
		if seenRuleNames[rule.Name] {
			return fmt.Errorf("duplicate pick rule name %q", rule.Name)
		}
		seenRuleNames[rule.Name] = true
	}
	if err := validateIDs("naming set", len(p.Naming), func(i int) string { return p.Naming[i].ID }); err != nil {
		return err
	}
	for i := range p.Naming {
		if err := p.Naming[i].validate(); err != nil {
			return fmt.Errorf("naming[%d]: %w", i, err)
		}
	}
	if err := validateIDs("release test", len(p.ReleaseTests), func(i int) string { return p.ReleaseTests[i].ID }); err != nil {
		return err
	}
	for i := range p.ReleaseTests {
		if err := p.ReleaseTests[i].validate(); err != nil {
			return fmt.Errorf("release_tests[%d]: %w", i, err)
		}
	}
	canonical, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("encode data pack: %w", err)
	}
	if len(canonical) > MaxBytes {
		return ErrTooLarge
	}
	return nil
}

func (r PickRule) validate() error {
	if err := validID("pick rule", r.ID); err != nil {
		return err
	}
	if err := mediafiles.CheckPickRules(r.Name, r.Kinds, r.Rules); err != nil {
		var fe *mediafiles.FieldError
		if errors.As(err, &fe) {
			return fmt.Errorf("%s: %s", fe.Field, fe.Text)
		}
		return err
	}
	// Your own traits live on one install only; a data pack cannot carry them.
	keys := append(append([]string{}, r.Rules.MustHave...), r.Rules.Never...)
	for _, tp := range r.Rules.Traits {
		keys = append(keys, tp.Trait)
	}
	for _, k := range keys {
		if strings.HasPrefix(k, mediafiles.OwnTraitPrefix) {
			return fmt.Errorf("pick rule %q uses %s, a trait of one install's own; a data pack cannot carry it", r.Name, k)
		}
	}
	return nil
}

func (n NamingSet) validate() error {
	if err := validID("naming set", n.ID); err != nil {
		return err
	}
	if err := validName("naming set", n.Name); err != nil {
		return err
	}
	p := n.Patterns
	patterns := []string{p.FilePattern, p.FolderPattern, p.ShowFolderPattern, p.SeasonFolderPattern, p.SpecialsFolder, p.EpisodePattern,
		p.DailyPattern, p.AnimePattern, p.ArtistFolderPattern, p.AlbumFolderPattern, p.TrackPattern, p.MultiDiscTrackPattern, p.DiscFolderPattern}
	for _, value := range patterns {
		if strings.TrimSpace(value) == "" || utf8.RuneCountInString(value) > maxText {
			return errors.New("every naming pattern must hold 1 to 200 characters")
		}
	}
	s := mediafiles.NamingDefaults()
	s.FilePattern, s.FolderPattern = p.FilePattern, p.FolderPattern
	s.ShowFolderPattern, s.SeasonFolderPattern, s.SpecialsFolder = p.ShowFolderPattern, p.SeasonFolderPattern, p.SpecialsFolder
	s.EpisodePattern, s.DailyPattern, s.AnimePattern = p.EpisodePattern, p.DailyPattern, p.AnimePattern
	s.ArtistFolderPattern, s.AlbumFolderPattern = p.ArtistFolderPattern, p.AlbumFolderPattern
	s.TrackPattern, s.MultiDiscTrackPattern, s.DiscFolderPattern = p.TrackPattern, p.MultiDiscTrackPattern, p.DiscFolderPattern
	s.Colon, s.Illegal = p.Colon, p.Illegal
	s.MultiEpisodeStyle, s.AirDateStyle, s.AbsoluteDigits = p.MultiEpisodeStyle, p.AirDateStyle, p.AbsoluteDigits
	if err := s.Validate(); err != nil {
		return fmt.Errorf("naming patterns: %w", err)
	}
	return nil
}

func (r ReleaseTest) validate() error {
	if err := validID("release test", r.ID); err != nil {
		return err
	}
	if err := validName("release test", r.Name); err != nil {
		return err
	}
	if r.ReleaseName == "" || len(r.ReleaseName) > maxReleaseName || !utf8.ValidString(r.ReleaseName) {
		return errors.New("release_name must hold 1 to 512 UTF-8 bytes")
	}
	if r.Hints.Show && r.Hints.Music {
		return errors.New("release hints cannot be both show and music")
	}
	if !knownQuality(r.Expected.Quality) {
		return fmt.Errorf("expected quality %q is unknown", r.Expected.Quality)
	}
	p := releasename.Parse(r.ReleaseName, releasename.Hints{Show: r.Hints.Show, Music: r.Hints.Music})
	actual := ExpectedFacts{Kind: p.Kind.Value, Quality: mediafiles.StepFor(p), Resolution: p.Resolution.Value,
		Source: p.Source.Value, Codec: p.Codec.Value, Flags: p.Flags}
	if !equalFacts(actual, r.Expected) {
		return fmt.Errorf("release facts mismatch: got %+v, expected %+v", actual, r.Expected)
	}
	return nil
}

func equalFacts(a, b ExpectedFacts) bool {
	return a.Kind == b.Kind && a.Quality == b.Quality && a.Resolution == b.Resolution && a.Source == b.Source && a.Codec == b.Codec && slices.Equal(a.Flags, b.Flags)
}

func knownQuality(q string) bool {
	return slices.Contains(mediafiles.Ladder, q) || slices.Contains(mediafiles.MusicLadder, q)
}

func validID(kind, id string) error {
	if !idRE.MatchString(id) {
		return fmt.Errorf("%s id %q must be 1 to 64 lowercase ASCII letters, digits, dots, dashes or underscores", kind, id)
	}
	return nil
}

func validName(kind, name string) error {
	if strings.TrimSpace(name) == "" || utf8.RuneCountInString(name) > maxText || !utf8.ValidString(name) {
		return fmt.Errorf("%s name must hold 1 to 200 UTF-8 characters", kind)
	}
	return nil
}

func validateIDs(kind string, n int, at func(int) string) error {
	seen := map[string]bool{}
	for i := 0; i < n; i++ {
		id := at(i)
		if seen[id] {
			return fmt.Errorf("duplicate %s id %q", kind, id)
		}
		seen[id] = true
	}
	return nil
}

func rejectDuplicateKeys(raw []byte) error {
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	if err := scanValue(d); err != nil {
		return fmt.Errorf("decode data pack: %w", err)
	}
	if err := requireEOF(d); err != nil {
		return err
	}
	return nil
}

func scanValue(d *json.Decoder) error {
	tok, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := tok.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]bool{}
		for d.More() {
			keyTok, err := d.Token()
			if err != nil {
				return err
			}
			key, ok := keyTok.(string)
			if !ok {
				return errors.New("object key is not a string")
			}
			if seen[key] {
				return fmt.Errorf("duplicate JSON key %q", key)
			}
			seen[key] = true
			if err := scanValue(d); err != nil {
				return err
			}
		}
		_, err = d.Token()
		return err
	case '[':
		for d.More() {
			if err := scanValue(d); err != nil {
				return err
			}
		}
		_, err = d.Token()
		return err
	default:
		return errors.New("unexpected JSON delimiter")
	}
}

func requireEOF(d *json.Decoder) error {
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			return errors.New("data pack has a trailing JSON value")
		}
		return fmt.Errorf("decode data pack: %w", err)
	}
	return nil
}

// Import validates everything before storing one inactive pack in one write.
func Import(ctx context.Context, st *database.Store, raw []byte, by domain.Actor) (Pack, error) {
	p, err := Decode(raw)
	if err != nil {
		if errors.Is(err, ErrTooLarge) {
			return Pack{}, err
		}
		return Pack{}, invalidInput(err)
	}
	canonical, err := Encode(p)
	if err != nil {
		if errors.Is(err, ErrTooLarge) {
			return Pack{}, err
		}
		return Pack{}, invalidInput(err)
	}
	actor, _ := json.Marshal(by)
	err = st.Write(ctx, func(tx *sql.Tx) error {
		key := "data_pack:" + p.ID
		var exists int
		err := tx.QueryRowContext(ctx, `SELECT 1 FROM settings WHERE grp = 'this_app' AND key = ?`, key).Scan(&exists)
		if err == nil {
			return ErrConflict
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		now := database.FormatTime(time.Now())
		_, err = tx.ExecContext(ctx, `INSERT INTO settings (grp, key, value, created_at, created_by, updated_at, updated_by) VALUES ('this_app', ?, ?, ?, ?, ?, ?)`,
			key, string(canonical), now, string(actor), now, string(actor))
		return err
	})
	if err != nil {
		return Pack{}, err
	}
	return p, nil
}

func List(ctx context.Context, st *database.Store) ([]Summary, error) {
	rows, err := st.Read().QueryContext(ctx, `SELECT value FROM settings WHERE grp = 'this_app' AND key GLOB 'data_pack:*' ORDER BY key`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Summary
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			return nil, err
		}
		p, err := Decode([]byte(raw))
		if err != nil {
			return nil, fmt.Errorf("stored data pack is invalid: %w", err)
		}
		out = append(out, Summary{ID: p.ID, Name: p.Name, Status: "inactive", PickRules: len(p.PickRules), NamingSets: len(p.Naming), ReleaseTests: len(p.ReleaseTests)})
	}
	return out, rows.Err()
}

func Export(ctx context.Context, st *database.Store, id string) ([]byte, error) {
	if err := validID("data pack", id); err != nil {
		return nil, invalidInput(err)
	}
	var raw string
	err := st.Read().QueryRowContext(ctx, `SELECT value FROM settings WHERE grp = 'this_app' AND key = ?`, "data_pack:"+id).Scan(&raw)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	p, err := Decode([]byte(raw))
	if err != nil {
		return nil, fmt.Errorf("stored data pack is invalid: %w", err)
	}
	return Encode(p)
}
