// Package srctag records where every stored music value came from.
//
// Values copied from a MusicBrainz dump carry one tag per table
// (dump_meta: the dump date, its schema sequence and the row count), not
// one per row. Values that override or add to the dump (Deezer, Cover Art
// Archive, Apple Music, spotify81, ListenBrainz, live MusicBrainz) carry a
// tag per value in source_tags. Picture-source matches keep theirs in the
// picture store's source_ids table. Report answers the counts per source
// over all three, and how many values have no source or no fetch date.
package srctag

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Tag says where one value came from: the source ("deezer", "caa",
// "applemusic", "spotify81", "listenbrainz", "mb-live"), the source's own
// reference for it (an id or URL), when it was fetched and which egress
// address fetched it ("" for a direct call).
type Tag struct {
	Source    string
	Ref       string
	FetchedAt time.Time
	ViaIP     string
}

// MBDump is the source of answers read from the slim MusicBrainz dump
// copy; their Ref is the dump date.
const MBDump = "mb-dump"

// ErrUntagged: a write came without a source.
var ErrUntagged = errors.New("srctag: value without a source")

// Check refuses a tag without a source.
func (t Tag) Check() error {
	if strings.TrimSpace(t.Source) == "" {
		return ErrUntagged
	}
	return nil
}

// At answers the fetch time, now when the tag has none.
func (t Tag) At(now time.Time) time.Time {
	if t.FetchedAt.IsZero() {
		return now
	}
	return t.FetchedAt
}

// Schema makes the dump_meta and source_tags tables.
const Schema = `CREATE TABLE IF NOT EXISTS dump_meta(
 tbl TEXT PRIMARY KEY, dump_date TEXT NOT NULL, schema_sequence INTEGER NOT NULL, rows INTEGER NOT NULL);
CREATE TABLE IF NOT EXISTS source_tags(
 entity TEXT NOT NULL, id TEXT NOT NULL, field TEXT NOT NULL,
 source TEXT NOT NULL, source_ref TEXT NOT NULL DEFAULT '',
 fetched_at INTEGER NOT NULL, via_ip TEXT NOT NULL DEFAULT '',
 PRIMARY KEY(entity, id, field));
CREATE INDEX IF NOT EXISTS source_tags_source ON source_tags(source);`

// Migrate makes the tables in db.
func Migrate(db *sql.DB) error {
	_, err := db.Exec(Schema)
	return err
}

// DumpMeta is the one tag a dump table carries.
type DumpMeta struct {
	Table          string
	DumpDate       string
	SchemaSequence int
	Rows           int64
}

type execer interface {
	ExecContext(ctx context.Context, q string, args ...any) (sql.Result, error)
}

// PutDump records a dump table's tag.
func PutDump(ctx context.Context, db execer, m DumpMeta) error {
	if m.Table == "" || m.DumpDate == "" || m.SchemaSequence <= 0 {
		return fmt.Errorf("srctag: incomplete dump tag %+v", m)
	}
	_, err := db.ExecContext(ctx, `INSERT INTO dump_meta(tbl, dump_date, schema_sequence, rows) VALUES(?,?,?,?)
ON CONFLICT(tbl) DO UPDATE SET dump_date=excluded.dump_date, schema_sequence=excluded.schema_sequence, rows=excluded.rows`,
		m.Table, m.DumpDate, m.SchemaSequence, m.Rows)
	return err
}

// Put records the source of one override value (entity, id, field).
func Put(ctx context.Context, db execer, entity, id, field string, t Tag, now time.Time) error {
	if err := t.Check(); err != nil {
		return err
	}
	_, err := db.ExecContext(ctx, `INSERT INTO source_tags(entity, id, field, source, source_ref, fetched_at, via_ip) VALUES(?,?,?,?,?,?,?)
ON CONFLICT(entity, id, field) DO UPDATE SET source=excluded.source, source_ref=excluded.source_ref, fetched_at=excluded.fetched_at, via_ip=excluded.via_ip`,
		entity, id, field, t.Source, t.Ref, t.At(now).Unix(), t.ViaIP)
	return err
}

// ReportSQL is the AC 2 query: counts per source over the dump tables,
// the override tags and the picture-source matches (attached as img),
// with the values missing a source or a fetch date counted as untagged.
const ReportSQL = `SELECT 'dump' AS layer, 'musicbrainz-dump:' || dump_date AS source, SUM(rows) AS n,
 SUM(CASE WHEN dump_date = '' OR schema_sequence <= 0 THEN rows ELSE 0 END) AS untagged
 FROM dump_meta GROUP BY dump_date
UNION ALL
SELECT 'override', source, COUNT(*), SUM(CASE WHEN source = '' OR fetched_at <= 0 THEN 1 ELSE 0 END)
 FROM source_tags GROUP BY source
UNION ALL
SELECT 'picture-id', source, COUNT(*), SUM(CASE WHEN source = '' OR fetched_at <= 0 THEN 1 ELSE 0 END)
 FROM img.source_ids GROUP BY source
ORDER BY 1, 2`

// Count is one line of the report.
type Count struct {
	Layer, Source string
	N, Untagged   int64
}

// Report runs ReportSQL on db with the picture store's database file
// attached read-only as img.
func Report(ctx context.Context, db *sql.DB, imgPath string) ([]Count, error) {
	conn, err := db.Conn(ctx)
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	if _, err := conn.ExecContext(ctx, `ATTACH DATABASE ? AS img`, "file:"+imgPath+"?mode=ro"); err != nil {
		return nil, err
	}
	defer conn.ExecContext(context.Background(), `DETACH DATABASE img`)
	rows, err := conn.QueryContext(ctx, ReportSQL)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Count
	for rows.Next() {
		var c Count
		if err := rows.Scan(&c.Layer, &c.Source, &c.N, &c.Untagged); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}
