package srctag

import (
	"database/sql"
	"strings"
)

// Untagged is the source given to rows stored before values were tagged,
// when the source is not known. They count as untagged until written
// again.
const Untagged = "unknown-pre-tag"

// Spec says how one store table carries its tag: a src column and a
// fetched_at column kept by triggers, so every write that stores a body
// names its source and fetch time.
type Spec struct {
	Table string
	Key   []string // primary key columns
	Body  string   // the column holding the value; empty or NULL is not a stored value
	// Src is the SQL expression for the source of a new write; "@." stands
	// for the row (NEW. in the trigger, nothing in the backfill).
	Src string
	// Back is the expression for rows stored before tagging (default Src).
	Back string
	// Known is the expression for those rows' fetch time (unix seconds),
	// or "" when it is not known (0, counted as untagged).
	Known string
	// Empty, when set, is the source of a row whose body is empty: a real
	// answer that the source has nothing (a "none" row). Such rows are
	// tagged too, on write and in the backfill. "": empty rows are no
	// stored value and stay untagged.
	Empty string
}

// Stamp adds the src and fetched_at columns to the table when missing,
// creates the triggers that fill them on every write of a body, and tags
// the rows stored before with what is known (never a guess: an unknown
// source is Untagged, an unknown time is 0).
func Stamp(db *sql.DB, s Spec) error {
	for _, col := range []string{"src TEXT NOT NULL DEFAULT ''", "fetched_at INTEGER NOT NULL DEFAULT 0"} {
		if _, err := db.Exec(`ALTER TABLE ` + s.Table + ` ADD COLUMN ` + col); err != nil && !strings.Contains(err.Error(), "duplicate column") {
			return err
		}
	}
	where := make([]string, len(s.Key))
	for i, k := range s.Key {
		where[i] = k + "=NEW." + k
	}
	body := func(prefix string) string { return `length(` + prefix + s.Body + `)>0` }
	src := func(expr, prefix string) string {
		if s.Empty == "" {
			return strings.ReplaceAll(expr, "@.", prefix)
		}
		return `CASE WHEN ` + body(prefix) + ` THEN ` + strings.ReplaceAll(expr, "@.", prefix) + ` ELSE ` + strings.ReplaceAll(s.Empty, "@.", prefix) + ` END`
	}
	when := func(prefix string) string {
		if s.Empty != "" {
			return "1"
		}
		return body(prefix)
	}
	set := `UPDATE ` + s.Table + ` SET src=` + src(s.Src, "NEW.") + `, fetched_at=CAST(strftime('%s','now') AS INTEGER) WHERE ` + strings.Join(where, " AND ") + `; END`
	// The triggers are dropped and made again in one transaction, so no
	// write in between is left untagged.
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	for _, t := range []struct{ name, ev string }{{"srctag_i_", "AFTER INSERT"}, {"srctag_u_", "AFTER UPDATE OF " + s.Body}} {
		if _, err := tx.Exec(`DROP TRIGGER IF EXISTS ` + t.name + s.Table + `; CREATE TRIGGER ` + t.name + s.Table + ` ` + t.ev + ` ON ` + s.Table + ` WHEN ` + when("NEW.") + ` BEGIN ` + set); err != nil {
			tx.Rollback()
			return err
		}
	}
	if err := tx.Commit(); err != nil {
		return err
	}
	back, known := s.Back, s.Known
	if back == "" {
		back = s.Src
	}
	if known == "" {
		known = "0"
	}
	_, err = db.Exec(`UPDATE ` + s.Table + ` SET src=` + src(back, "") + `, fetched_at=` + strings.ReplaceAll(known, "@.", "") + ` WHERE src='' AND ` + when(""))
	return err
}
