package migrate

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
)

// MaxSnapshot is the largest Snapshot kept (its JSON, in bytes).
var MaxSnapshot = 16 << 20

// MaxSnapshots is how many Snapshots are kept; the oldest goes first.
const MaxSnapshots = 50

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
	data, err := json.Marshal(s)
	if err != nil {
		return Saved{}, err
	}
	if len(data) > MaxSnapshot {
		return Saved{}, ErrSnapshotTooBig
	}
	counts := s.Counts()
	cj, _ := json.Marshal(counts)
	row := Saved{CreatedAt: time.Now().UTC().Truncate(time.Second), OwnerID: owner, Kind: s.Kind, Via: s.Via, Address: s.Address,
		AppVersion: s.Version, Counts: counts, Size: len(data)}
	err = func() error {
		res, err := tx.ExecContext(ctx, `INSERT INTO migrate_snapshots (created_at, owner_id, app_kind, via, address, app_version, counts, size, data)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`, row.CreatedAt.Format(time.RFC3339), owner, string(s.Kind), s.Via, s.Address, s.Version, string(cj), len(data), string(data))
		if err != nil {
			return err
		}
		row.ID, err = res.LastInsertId()
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `DELETE FROM migrate_snapshots WHERE id NOT IN
			(SELECT id FROM migrate_snapshots ORDER BY id DESC LIMIT ?)`, MaxSnapshots)
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
	var data string
	s, err := scanSaved(st.Read().QueryRowContext(ctx, `SELECT `+savedCols+`, data FROM migrate_snapshots WHERE id = ?`, id), &data)
	if errors.Is(err, sql.ErrNoRows) {
		return Saved{}, nil, ErrNoSnapshot
	}
	if err != nil {
		return Saved{}, nil, err
	}
	var snap Snapshot
	if err := json.Unmarshal([]byte(data), &snap); err != nil {
		return Saved{}, nil, err
	}
	return s, &snap, nil
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
