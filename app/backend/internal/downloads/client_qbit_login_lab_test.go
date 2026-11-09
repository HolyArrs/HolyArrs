//go:build lab

package downloads

import (
	"context"
	"os"
	"testing"
)

func TestLabQbitLogin(t *testing.T) {
	user, pass := os.Getenv("HOLYARRS_LAB_QBIT_USER"), os.Getenv("HOLYARRS_LAB_QBIT_PASS")
	if user == "" || pass == "" {
		t.Skip("HOLYARRS_LAB_QBIT_USER/PASS not set (run through labctl qbt-login)")
	}
	base := os.Getenv("HOLYARRS_LAB_QBIT")
	if base == "" {
		base = "http://127.0.0.1:18708"
	}
	ctx := context.Background()
	q := NewQbittorrentLogin(base, user, pass, nil)
	if err := q.signIn(ctx); err != nil {
		t.Fatalf("sign-in: %v", err)
	}
	h, err := q.Health(ctx)
	if err != nil {
		t.Fatalf("health: %v", err)
	}
	if _, err := q.List(ctx, "holyarrs-labtest"); err != nil {
		t.Fatalf("list: %v", err)
	}
	st, err := q.Status(ctx)
	if err != nil || st.FreeSpace <= 0 {
		t.Fatalf("status %+v %v", st, err)
	}
	t.Logf("signed in to qBittorrent %s with user name and password; free space %d", h.Version, st.FreeSpace)
}
