package library

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func advScan(t *testing.T, batch bool, folders []string, after string) (AdoptPlan, int64, int64, error) {
	t.Helper()
	s := open(t)
	lf := addFolder(t, s)
	for _, folder := range folders {
		dir := filepath.Join(lf.Path, filepath.FromSlash(folder))
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, "f.mkv"), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	base, b, sg := movieServer(t, batch)
	md := &metadata.TMDB{BaseURL: base + metadata.TMDBPath, Access: func(context.Context) (string, error) { return "tok", nil }, HTTP: http.DefaultClient, Gap: 1}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	plan, err := PlanAdoptFrom(ctx, s.Read(), md, lf.ID, after)
	return plan, b.Load(), sg.Load(), err
}

func TestAdvDuplicateNames(t *testing.T) {
	filler := func(prefix string, from, n int) []string {
		var out []string
		for i := from; i < from+n; i++ {
			out = append(out, fmt.Sprintf("%s/Film %d (2001)", prefix, i))
		}
		return out
	}
	five := []string{"A/Film 1 (2001)"}
	five = append(five, filler("B", 2, 2*scanWindow)...)
	five = append(five, "C/Film 1 (2001)")
	five = append(five, filler("D", 500, 2*scanWindow)...)
	five = append(five, "E/Film 1 (2001)", "F/Film 1 (2001)")
	five = append(five, filler("G", 900, 2*scanWindow)...)
	five = append(five, "Z/Film 1 (2001)")
	var same []string
	for i := range 3 * scanWindow {
		same = append(same, fmt.Sprintf("G%03d/Film 1 (2001)", i))
	}
	top := append([]string{"Film 1 (2001)"}, filler("M", 2, 3*scanWindow)...)
	top = append(top, "Z/Film 1 (2001)")
	cases := map[string][]string{"five copies": five, "all same name": same, "top level and nested": top}
	for name, folders := range cases {
		for _, batch := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/batch=%v", name, batch), func(t *testing.T) {
				start := time.Now()
				plan, b, sg, err := advScan(t, batch, folders, "")
				if err != nil {
					t.Fatalf("err %v after %v", err, time.Since(start))
				}
				if len(plan.Folders) != min(len(folders), plan.Limit) || plan.Counts["matched"] != len(plan.Folders) {
					t.Fatalf("got %d folders %v", len(plan.Folders), plan.Counts)
				}
				t.Logf("folders %d batches %d singles %d in %v", len(plan.Folders), b, sg, time.Since(start))
			})
		}
	}
	// Resume after a folder between two copies.
	t.Run("resume", func(t *testing.T) {
		plan, _, _, err := advScan(t, true, five, five[scanWindow])
		if err != nil {
			t.Fatal(err)
		}
		if plan.Counts["matched"] != len(plan.Folders) || len(plan.Folders) == 0 {
			t.Fatalf("%v", plan.Counts)
		}
	})
}

// Unit level: duplicate keys fed straight to startAhead, and a jump over
// several windows (wait skipped for folders in between).
func TestAdvAheadDirect(t *testing.T) {
	var folders []string
	for i := range 6 * scanWindow {
		folders = append(folders, fmt.Sprintf("Film %d (2001)", i))
	}
	t.Run("jump", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		a := startAhead(ctx, advPrefetch{}, folders)
		defer a.close()
		a.wait(ctx, folders[0])
		a.wait(ctx, folders[4*scanWindow])
		for _, f := range folders[4*scanWindow:] {
			a.wait(ctx, f)
		}
		if ctx.Err() != nil {
			t.Fatal("stalled after jump")
		}
	})
	t.Run("dup keys", func(t *testing.T) {
		fs := append([]string(nil), folders...)
		fs[3], fs[5*scanWindow] = "movies", "movies"
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		a := startAhead(ctx, advPrefetch{}, fs)
		defer a.close()
		for _, f := range fs {
			a.wait(ctx, f)
		}
		if ctx.Err() != nil {
			t.Fatal("stalled on dup keys")
		}
	})
}

type advPrefetch struct{ metadata.Provider }

func (advPrefetch) Prefetch(context.Context, []metadata.TMDBRead) int { return 0 }
