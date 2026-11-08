package migrate

import (
	"context"
	"encoding/json"
	"testing"
)

func TestPlanHashStableAcrossSourceSettings(t *testing.T) {
	snapshot := &Snapshot{Kind: Radarr, Via: ViaAPI, Indexers: []Provider{{
		Name: "fixture", Kind: "Torznab", Settings: map[string]any{
			"baseUrl": "https://source.example.invalid/", "alpha": true, "beta": 2,
			"gamma": "value", "delta": false, "epsilon": 5, "zeta": "value",
		},
	}}}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	want := PlanHash(BuildPlan([]Input{{ID: 1, Snap: snapshot}}, Choices{}))
	for i := 0; i < 100; i++ {
		var restored Snapshot
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatal(err)
		}
		for _, candidate := range []*Snapshot{snapshot, &restored} {
			if got := PlanHash(BuildPlan([]Input{{ID: 1, Snap: candidate}}, Choices{})); got != want {
				t.Fatal("unchanged source settings changed the approval hash")
			}
		}
	}
}

func TestMoveAcceptsUnchangedReviewedSourceSettings(t *testing.T) {
	lab := newMoveLabWith(t, func(radarr, bazarr, seerr *Snapshot) {
		for _, key := range []string{"alpha", "beta", "gamma", "delta", "epsilon", "zeta"} {
			radarr.Indexers[0].Settings[key] = true
		}
		radarr.Folders[0].Extra = map[string]any{"alpha": true, "beta": 2, "gamma": "value", "delta": false, "epsilon": 5, "zeta": "value"}
	})
	ctx := context.Background()
	choices := Choices{PathRules: lab.rules, People: lab.people}
	plan, err := LoadPlan(ctx, lab.st, lab.snaps, choices)
	if err != nil {
		t.Fatal(err)
	}
	reviewed := PlanHash(plan)
	for i := 0; i < 30; i++ {
		next, err := LoadPlan(ctx, lab.st, lab.snaps, choices)
		if err != nil {
			t.Fatal(err)
		}
		if PlanHash(next) != reviewed {
			t.Fatal("saved plan no longer matches the reviewed approval hash")
		}
	}
}

func TestPlanHashStableAcrossFolderExtras(t *testing.T) {
	snapshot := &Snapshot{Kind: Radarr, Via: ViaAPI, Folders: []Folder{{Path: "/movies", Extra: map[string]any{
		"alpha": true, "beta": 2, "gamma": "value", "delta": false, "epsilon": 5, "zeta": "value",
	}}}}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	want := PlanHash(BuildPlan([]Input{{ID: 1, Snap: snapshot}}, Choices{}))
	for i := 0; i < 100; i++ {
		var restored Snapshot
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatal(err)
		}
		if PlanHash(BuildPlan([]Input{{ID: 1, Snap: &restored}}, Choices{})) != want {
			t.Fatal("unchanged folder details changed the approval hash")
		}
	}
}

func TestPlanHashStableAcrossUnknownMetadataProfileFields(t *testing.T) {
	snapshot := &Snapshot{Kind: Lidarr,
		Folders: []Folder{{Path: "/music"}},
		MetadataProfiles: []Named{{ID: 7, Name: "Profile", Settings: map[string]any{
			"primaryalbumtypes":   []any{map[string]any{"name": "Album", "allowed": true}},
			"secondaryalbumtypes": []any{map[string]any{"name": "Studio", "allowed": true}},
			"releasestatuses":     []any{map[string]any{"name": "Official", "allowed": true}},
			"zeta":                "last", "alpha": "first",
		}}},
		Titles: []Title{{SourceID: 1, Kind: "artist", Name: "Artist", Path: "/music/Artist", Monitored: true,
			IDs:   map[string]string{"musicbrainz": "056e4f3e-d505-4dad-8ec1-d04f521cbb56"},
			Extra: map[string]any{"new_albums": "new", "metadata_profile_id": int64(7)}}},
	}
	data, err := json.Marshal(snapshot)
	if err != nil {
		t.Fatal(err)
	}
	choices := Choices{PathRules: []PathRule{{From: "/music", To: "C:/music"}}}
	want := PlanHash(BuildPlan([]Input{{ID: 1, Snap: snapshot}}, choices))
	for i := 0; i < 100; i++ {
		var restored Snapshot
		if err := json.Unmarshal(data, &restored); err != nil {
			t.Fatal(err)
		}
		if got := PlanHash(BuildPlan([]Input{{ID: 1, Snap: &restored}}, choices)); got != want {
			t.Fatalf("metadata settings changed the approval hash: %s != %s", got, want)
		}
	}
}
