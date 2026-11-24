package main

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"
	"github.com/HolyArrs/holyarrs/app/backend/internal/sources"
)

// route: POST /api/v1/releases/{id}/turn-down
func TestTurnDownBlocksGroup(t *testing.T) {
	t.Parallel()
	u, s, _, _, slotID, rels := searchWorld(t)
	ctx := context.Background()
	rulesOf := func() mediafiles.PickRules {
		t.Helper()
		var sl mediafiles.Slot
		var ver mediafiles.Version
		var pr mediafiles.PickRules
		if err := mediafiles.LoadSlot(ctx, s.Read(), slotID, &sl); err != nil {
			t.Fatal(err)
		}
		if err := mediafiles.LoadVersion(ctx, s.Read(), sl.VersionID, &ver); err != nil {
			t.Fatal(err)
		}
		if err := mediafiles.LoadPickRules(ctx, s.Read(), ver.PickRulesID, &pr); err != nil {
			t.Fatal(err)
		}
		return pr
	}
	before := rulesOf()

	var tv sources.ReleaseView
	_ = json.Unmarshal(do(t, "POST", u+"/api/v1/releases/"+itoa(rels["passed_over"])+"/turn-down", `{"block_group":true}`, 200), &tv)
	if tv.State != "turned_down" {
		t.Fatalf("turned down %+v", tv)
	}
	after := rulesOf()
	if len(after.Body.BlockedGroups) != 1 || after.Body.BlockedGroups[0] != "OTHER" || after.Revision != before.Revision+1 {
		t.Fatalf("blocked groups %q revision %d -> %d", after.Body.BlockedGroups, before.Revision, after.Revision)
	}

	// The same group again, in any case, adds nothing.
	do(t, "POST", u+"/api/v1/releases/"+itoa(rels["passed_over"])+"/turn-down", `{"block_group":true}`, 200)
	if again := rulesOf(); len(again.Body.BlockedGroups) != 1 || again.Revision != after.Revision {
		t.Fatalf("blocked twice %q", again.Body.BlockedGroups)
	}
	do(t, "POST", u+"/api/v1/releases/999/turn-down", `{"block_group":true}`, 404)
}
