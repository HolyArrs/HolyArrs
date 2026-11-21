package downloads

import (
	"context"
	"database/sql"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// A readwrite key hides slskd's settings; the path pair's slskd side stands in, through the real Check path.
func TestSlskdReadwriteKeyPathPairCheck(t *testing.T) {
	w, m := setup(t)
	ctx := context.Background()
	u := startSlskd(t, &fakeSlskd{adminOnly: true})
	m.FromVault = func(context.Context, int64) (string, error) { return slskdKey, nil }
	var ref int64
	if err := w.s.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec(`INSERT INTO secrets (kind, value, created_at, created_by, updated_at, updated_by) VALUES ('downloader_key', 'sealed', '', '{}', '', '{}')`)
		if err != nil {
			return err
		}
		ref, err = res.LastInsertId()
		return err
	}); err != nil {
		t.Fatal(err)
	}
	d := &domain.Downloader{Name: "slskd", Adapter: "slskd", Protocol: "soulseek", URL: u, Category: "music", Priority: 1, AllowLocal: true, SecretRef: &ref,
		PathMap: []domain.PathMap{{Theirs: "/app/downloads", Ours: "/slskd/downloads"}}}
	w.addDownloader(t, d)
	if _, err := m.CheckNow(ctx, *d); err != nil {
		t.Fatalf("check: %v", err)
	}
}
