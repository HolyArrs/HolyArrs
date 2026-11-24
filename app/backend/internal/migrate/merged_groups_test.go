package migrate

import (
	"encoding/json"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/library"
)

// A TRaSH "Merged QPs" profile as Radarr's API sends it: one allowed group
// holding WEB and Blu-ray at 720p and 1080p, Remux on its own above it.
const mergedProfileJSON = `{"id":9,"name":"German HD","upgradeAllowed":true,"cutoff":1001,
"minFormatScore":0,"cutoffFormatScore":10000,"minUpgradeFormatScore":1,
"items":[
 {"quality":{"id":1,"name":"SDTV"},"items":[],"allowed":false},
 {"id":1001,"name":"Merged QPs","allowed":true,"items":[
  {"quality":{"id":14,"name":"WEBRip-720p"},"items":[],"allowed":true},
  {"quality":{"id":5,"name":"WEBDL-720p"},"items":[],"allowed":true},
  {"quality":{"id":6,"name":"Bluray-720p"},"items":[],"allowed":true},
  {"quality":{"id":15,"name":"WEBRip-1080p"},"items":[],"allowed":true},
  {"quality":{"id":3,"name":"WEBDL-1080p"},"items":[],"allowed":true},
  {"quality":{"id":7,"name":"Bluray-1080p"},"items":[],"allowed":true}]},
 {"quality":{"id":30,"name":"Remux-1080p"},"items":[],"allowed":true}]}`

func TestProfileKeepsMergedGroups(t *testing.T) {
	var m map[string]any
	if err := json.Unmarshal([]byte(mergedProfileJSON), &m); err != nil {
		t.Fatal(err)
	}
	p := profile(lower(m), nil, nil)
	if len(p.Groups) != 1 || len(p.Groups[0]) != 6 {
		t.Fatalf("groups %v", p.Groups)
	}
	body, _ := library.OldProfileRules(library.OldProfile{Qualities: p.Qualities, CutoffQualities: p.CutoffQualities,
		Groups: p.Groups, UpgradeAllowed: p.UpgradeAllowed})
	if body.Order != "quality_then_points" || len(body.EqualSteps) != 1 || len(body.EqualSteps[0]) != 4 {
		t.Fatalf("order %q, equal steps %v", body.Order, body.EqualSteps)
	}
}
