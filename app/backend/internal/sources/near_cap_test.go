package sources

import (
	"context"
	"strings"
	"testing"
	"time"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

// TestNearCap_Limits checks when a Source counts as close to its search or pick limit.
func TestNearCap_Limits(t *testing.T) {
	ctx := context.Background()
	s, v := openVault(t)
	src, err := Add(ctx, s, v, Input{Name: "Lab", Format: "torznab", URL: "http://lab.example/api"}, domain.System)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	if near, _ := NearCap(ctx, s, src, now); near {
		t.Fatal("no limit is never near")
	}
	src.Limits = domain.SourceLimits{SearchesPerWindow: 5, WindowSeconds: 3600}
	for i := 0; i < 3; i++ {
		Record(ctx, s, src.ID, "search", time.Millisecond, nil)
	}
	if near, _ := NearCap(ctx, s, src, now.Add(time.Second)); near {
		t.Fatal("3 of 5 is not near")
	}
	Record(ctx, s, src.ID, "search", time.Millisecond, nil)
	near, why := NearCap(ctx, s, src, now.Add(time.Second))
	if !near || !strings.Contains(why, "search limit (4 of 5)") {
		t.Fatalf("4 of 5 should be near, got %v %q", near, why)
	}
	if near, _ := NearCap(ctx, s, src, now.Add(2*time.Hour)); near {
		t.Fatal("calls outside the window do not count")
	}
}
