package shows

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// A show folder with nothing in it is still listed (as no_video), and its
// videos reach the page as an empty list. A null there broke the import
// page, which reads the first video of every folder.
func TestPlanShowAdoptEmptyFolderHasAnEmptyVideoList(t *testing.T) {
	svc, s, _, _ := addShowService(t)
	lf := addShowFolder(t, s)
	if err := os.MkdirAll(filepath.Join(lf.Path, "Lab Show (2025)"), 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := PlanShowAdoptFrom(context.Background(), s.Read(), svc.Shows, lf.ID, "")
	if err != nil {
		t.Fatal(err)
	}
	c, ok := byFolder(p)["Lab Show (2025)"]
	if !ok || c.Status != "no_video" {
		t.Fatalf("empty folder: listed %v, %+v, want status no_video", ok, c)
	}
	if c.Videos == nil {
		t.Fatal("videos is nil, want an empty list")
	}
	b, err := json.Marshal(c)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"videos":[]`) {
		t.Fatalf("sent %s, want \"videos\":[]", b)
	}
}
