package migrate

import (
	"bytes"
	"compress/gzip"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// MaxSnapshot is the largest Snapshot kept (its compressed JSON, in bytes).
// A Sonarr library with many thousands of episode files makes a Snapshot past
// 16 MB of JSON; kept compressed it is about a tenth of that.
var MaxSnapshot = 64 << 20

// MaxSnapshots is how many Snapshots are kept; the oldest goes first.
const MaxSnapshots = 50

// maxSnapshotJSON bounds a kept Snapshot's JSON once unpacked.
const maxSnapshotJSON = 2 << 30

// MaxSnapshotsSize is the most all kept Snapshots hold together (compressed bytes); the
// oldest go first, and the newest is always kept.
var MaxSnapshotsSize = 256 << 20

// ErrSnapshotTooBig refuses a Snapshot over MaxSnapshot.
var ErrSnapshotTooBig = errors.New("the snapshot is bigger than the import tool keeps")

// ErrNoSnapshot is a missing Snapshot.
var ErrNoSnapshot = errors.New("no such snapshot")

// Saved is a kept Snapshot's row, without its data.
type Saved struct {
	ID         int64          `json:"id"`
	CreatedAt  time.Time      `json:"created_at"`
	OwnerID    int64          `json:"owner_id"`
	Kind       Kind           `json:"kind"`
	Via        string         `json:"via"`
	Address    string         `json:"address"`
	AppVersion string         `json:"app_version"`
	Counts     map[string]int `json:"counts"`
	Size       int            `json:"size"`
}

// Save keeps a Snapshot for owner and returns its row.
func Save(ctx context.Context, st *database.Store, owner int64, s *Snapshot) (Saved, error) {
	var row Saved
	err := st.Write(ctx, func(tx *sql.Tx) error {
		var err error
		row, err = SaveTx(ctx, tx, owner, s)
		return err
	})
	return row, err
}

// SaveTx keeps and prunes snapshots in the caller's transaction, so a
// one-use credential can be consumed atomically with its successful read.
func SaveTx(ctx context.Context, tx *sql.Tx, owner int64, s *Snapshot) (Saved, error) {
	// Kept gzip-compressed, written straight from the Snapshot so its JSON
	// is never held whole.
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if err := json.NewEncoder(zw).Encode(s); err != nil {
		return Saved{}, err
	}
	if err := zw.Close(); err != nil {
		return Saved{}, err
	}
	data := buf.Bytes()
	if len(data) > MaxSnapshot {
		return Saved{}, ErrSnapshotTooBig
	}
	counts := s.Counts()
	cj, _ := json.Marshal(counts)
	row := Saved{CreatedAt: time.Now().UTC().Truncate(time.Second), OwnerID: owner, Kind: s.Kind, Via: s.Via, Address: s.Address,
		AppVersion: s.Version, Counts: counts, Size: len(data)}
	err := func() error {
		// A new read never takes the number of a removed one an import still names.
		res, err := tx.ExecContext(ctx, `INSERT INTO migrate_snapshots (id, created_at, owner_id, app_kind, via, address, app_version, counts, size, data)
			VALUES (max(coalesce((SELECT max(id) FROM migrate_snapshots), 0), coalesce((SELECT max(CAST(j.value AS INTEGER)) FROM moves, json_each(moves.snapshots) j), 0)) + 1,
			?, ?, ?, ?, ?, ?, ?, ?, ?)`, row.CreatedAt.Format(time.RFC3339), owner, string(s.Kind), s.Via, s.Address, s.Version, string(cj), len(data), data)
		if err != nil {
			return err
		}
		row.ID, err = res.LastInsertId()
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM migrate_snapshots WHERE id NOT IN
			(SELECT id FROM migrate_snapshots ORDER BY id DESC LIMIT ?)`, MaxSnapshots)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM migrate_snapshots WHERE id < ? AND id IN
			(SELECT id FROM (SELECT id, sum(size) OVER (ORDER BY id DESC) AS total FROM migrate_snapshots) WHERE total > ?)`, row.ID, MaxSnapshotsSize)
		return err
	}()
	return row, err
}

const savedCols = `id, created_at, owner_id, app_kind, via, address, app_version, counts, size`

func scanSaved(sc interface{ Scan(...any) error }, extra ...any) (Saved, error) {
	var s Saved
	var created, counts string
	var kind string
	dest := append([]any{&s.ID, &created, &s.OwnerID, &kind, &s.Via, &s.Address, &s.AppVersion, &counts, &s.Size}, extra...)
	if err := sc.Scan(dest...); err != nil {
		return s, err
	}
	s.Kind = Kind(kind)
	s.CreatedAt, _ = time.Parse(time.RFC3339, created)
	_ = json.Unmarshal([]byte(counts), &s.Counts)
	return s, nil
}

// List lists the kept Snapshots, newest first.
func List(ctx context.Context, st *database.Store) ([]Saved, error) {
	rows, err := st.Read().QueryContext(ctx, `SELECT `+savedCols+` FROM migrate_snapshots ORDER BY id DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Saved{}
	for rows.Next() {
		s, err := scanSaved(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

// Get returns one kept Snapshot and its row.
func Get(ctx context.Context, st *database.Store, id int64) (Saved, *Snapshot, error) {
	var data []byte
	s, err := scanSaved(st.Read().QueryRowContext(ctx, `SELECT `+savedCols+`, data FROM migrate_snapshots WHERE id = ?`, id), &data)
	if errors.Is(err, sql.ErrNoRows) {
		return Saved{}, nil, ErrNoSnapshot
	}
	if err != nil {
		return Saved{}, nil, err
	}
	snap, err := decodeSnapshot(data)
	if err != nil {
		return Saved{}, nil, err
	}
	return s, snap, nil
}

// Delete removes one kept Snapshot.
func Delete(ctx context.Context, st *database.Store, id int64) error {
	return st.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.ExecContext(ctx, `DELETE FROM migrate_snapshots WHERE id = ?`, id)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNoSnapshot
		}
		return nil
	})
}

// snapshotUsed says whether an import names the read (yes when unsure).
func snapshotUsed(ctx context.Context, q database.Querier, id int64) bool {
	var n int
	err := q.QueryRowContext(ctx, `SELECT count(*) FROM moves, json_each(moves.snapshots) j WHERE CAST(j.value AS INTEGER) = ?`, id).Scan(&n)
	return err != nil || n > 0
}

// CountDoneMoves counts the finished moves.
func CountDoneMoves(ctx context.Context, q database.Querier) (int, error) {
	n := 0
	err := q.QueryRowContext(ctx, "SELECT count(*) FROM moves WHERE state = 'done'").Scan(&n)
	return n, err
}

// SnapshotKind names the old app a kept read came from, with its address
// ("" for a backup or a read kept before addresses were).
func SnapshotKind(ctx context.Context, q database.Querier, id int64) (kind, address string, err error) {
	err = q.QueryRowContext(ctx, "SELECT app_kind, address FROM migrate_snapshots WHERE id = ?", id).Scan(&kind, &address)
	return kind, address, err
}

// SetKeepUpToDateTx sets whether a title is kept up to date.
func SetKeepUpToDateTx(tx *sql.Tx, titleID int64, keep bool) error {
	_, err := tx.Exec(`UPDATE titles SET keep_up_to_date = ? WHERE id = ?`, keep, titleID)
	return err
}

// liveNamed is the id of the person here (not deleted) with this name, any
// case, or 0.
func liveNamed(ctx context.Context, db *sql.DB, name string) int64 {
	var id int64
	_ = db.QueryRowContext(ctx, "SELECT id FROM users WHERE name = ? COLLATE NOCASE AND removed_at = ''", name).Scan(&id)
	return id
}

// nameTaken says whether any row, a deleted person's included, holds name.
func nameTaken(ctx context.Context, db *sql.DB, name string) bool {
	var id int64
	return db.QueryRowContext(ctx, "SELECT id FROM users WHERE name = ? COLLATE NOCASE", name).Scan(&id) == nil
}

// decodeSnapshot reads a kept Snapshot: gzip-compressed JSON, or the plain
// JSON kept before 1.2.1.
func decodeSnapshot(data []byte) (*Snapshot, error) {
	var snap Snapshot
	if len(data) < 2 || data[0] != 0x1f || data[1] != 0x8b {
		return &snap, json.Unmarshal(data, &snap)
	}
	zr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer zr.Close()
	if err := json.NewDecoder(io.LimitReader(zr, maxSnapshotJSON)).Decode(&snap); err != nil {
		return nil, err
	}
	return &snap, nil
}

// importedDownloader is the id of the Downloader named name when an import
// made it, else 0.
func importedDownloader(ctx context.Context, db *sql.DB, name string) int64 {
	var id int64
	_ = db.QueryRowContext(ctx, `SELECT d.id FROM downloaders d WHERE d.removed = 0 AND d.name = ?
		AND EXISTS (SELECT 1 FROM move_made m WHERE m.what = 'downloader' AND m.row_id = d.id)`, name).Scan(&id)
	return id
}

// startQuickMove keeps a finished move with no report yet: the Downloaders
// added as soon as the old apps were read. It never forgets the move keys,
// which the full import still needs.
func startQuickMove(ctx context.Context, st *database.Store, owner int64, snaps []int64, at time.Time) (int64, error) {
	sj, _ := json.Marshal(snaps)
	var id int64
	err := st.Write(ctx, func(tx *sql.Tx) error {
		if err := busy(tx); err != nil {
			return err
		}
		res, err := tx.Exec("INSERT INTO moves (created_at, owner_id, snapshots, choices, state, finished_at) VALUES (?, ?, ?, '{}', 'done', ?)",
			at.Truncate(time.Second).Format(time.RFC3339), owner, string(sj), database.FormatTime(at))
		if err != nil {
			return err
		}
		id, err = res.LastInsertId()
		return err
	})
	return id, err
}

// finishQuickMove keeps the quick move's report.
func finishQuickMove(ctx context.Context, st *database.Store, id int64, rep *Report, at time.Time) error {
	b, _ := json.Marshal(rep)
	return st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE moves SET report = ?, finished_at = ? WHERE id = ?", string(b), database.FormatTime(at), id)
		return err
	})
}

// dropQuickMove removes a quick move that added nothing.
func dropQuickMove(ctx context.Context, st *database.Store, id int64) error {
	return st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("DELETE FROM moves WHERE id = ?", id)
		return err
	})
}

// madeCount is how many rows a move marked.
func madeCount(ctx context.Context, st *database.Store, id int64) (int, error) {
	var n int
	err := st.Read().QueryRowContext(ctx, "SELECT count(*) FROM move_made WHERE move_id = ?", id).Scan(&n)
	return n, err
}

// libraryFolderPaths lists the Library folder paths, oldest first; empty on a read error.
func libraryFolderPaths(ctx context.Context, st *database.Store) []string {
	var out []string
	rows, err := st.Read().QueryContext(ctx, "SELECT path FROM library_folders ORDER BY id")
	if err != nil {
		return out
	}
	defer rows.Close()
	for rows.Next() {
		var s string
		if rows.Scan(&s) == nil {
			out = append(out, s)
		}
	}
	return out
}
