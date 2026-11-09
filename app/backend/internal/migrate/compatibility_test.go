package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/core/jobs"
	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
)

func TestCompatibilityPlanKeepsLeastPrivilegeStableIdentityAndPerKindLimits(t *testing.T) {
	seerr := &Snapshot{Kind: Seerr, ReadAt: time.Date(2026, 10, 15, 0, 0, 0, 0, time.UTC),
		Users: []User{
			{SourceID: 11, ExternalID: "plex-9", Kind: "plex", Name: "Ana", Permissions: []string{"REQUEST", "AUTO_APPROVE_MOVIE", "REQUEST_4K"}, MovieLimit: 3, MovieDays: 7, ShowLimit: 2, ShowDays: 14},
			{SourceID: 12, Name: "Bo", Permissions: []string{"REQUEST_MOVIE", "AUTO_APPROVE_TV"}},
		},
		Requests: []Request{{SourceID: 21, Kind: "movie", IDs: map[string]string{"tmdb": "603"}, State: "approved", By: 11, DecidedBy: 12,
			Created: "2026-09-01T10:00:00Z", Updated: "2026-09-02T11:00:00Z"}},
		Settings: map[string]any{"default_movie_limit": int64(3), "default_movie_days": int64(7), "default_show_limit": int64(2), "default_show_days": int64(14),
			"default_permissions": []any{"REQUEST_MOVIE"}, "applicationTitle": "Ask here"},
	}
	p := BuildPlan([]Input{{ID: 9, Snap: seerr}}, Choices{})
	ana := item(t, p, GroupPeople, "person:seerr:plex:plex-9")
	if !slices.Equal(ana.Person.Perms, []string{"ask"}) {
		t.Fatalf("Ana permissions widened or lost: %+v", ana.Person.Perms)
	}
	if ana.Person.MoviesPerWeek == nil || *ana.Person.MoviesPerWeek != 3 || ana.Person.ShowsPerWeek != nil {
		t.Fatalf("per-kind limits not retained: %+v", ana.Person)
	}
	bo := item(t, p, GroupPeople, "person:seerr:12")
	if len(bo.Person.Perms) != 0 {
		t.Fatalf("kind-specific permission widened: %+v", bo.Person.Perms)
	}
	if _, ok := notMoved(p, "seerr users[1].permissions"); !ok {
		t.Fatal("unrepresentable scoped permissions were not reported")
	}
	ask := item(t, p, GroupRequests, "ask:seerr:21")
	if ask.Ask.By != ana.Key || ask.Ask.Decider != bo.Key || ask.Ask.DecidedAt != "2026-09-02T11:00:00Z" {
		t.Fatalf("request identities/timestamp not stable: %+v", ask.Ask)
	}
	defaults := item(t, p, GroupRequests, "wishlist_defaults")
	if defaults.AskSettings == nil || defaults.AskSettings.MoviesPerWeek == nil || *defaults.AskSettings.MoviesPerWeek != 3 || defaults.AskSettings.ShowsPerWeek != nil {
		t.Fatalf("supported defaults not mapped: %+v", defaults)
	}
	if _, ok := notMoved(p, "seerr settings.default_permissions"); !ok {
		t.Fatal("scoped default permissions were not retained as unsupported")
	}
}

func TestCompatibilityStablePersonChoiceNeverFallsBackToNumericAlias(t *testing.T) {
	s := &Snapshot{Kind: Seerr, Users: []User{{SourceID: 7, Kind: "plex", ExternalID: "stable-seven", Name: "Admin", Admin: true}}}
	stable := stablePersonKey(Seerr, s.Users[0])
	numeric := personKey(Seerr, 7)
	legacyChoice := BuildPlan([]Input{{ID: 1, Snap: s}}, Choices{People: map[string]string{numeric: PersonAdmin}})
	legacyItem := item(t, legacyChoice, GroupPeople, stable)
	if legacyItem.Person.Role != "member" {
		t.Fatal("numeric alias authorized a stable external identity")
	}
	stableChoice := BuildPlan([]Input{{ID: 1, Snap: s}}, Choices{People: map[string]string{stable: PersonAdmin}})
	stableItem := item(t, stableChoice, GroupPeople, stable)
	if stableItem.Person.Role != "admin" {
		t.Fatal("stable identity choice was not honored")
	}
}

func TestCompatibilityStableProviderIdentityIgnoresNumericIDChanges(t *testing.T) {
	first := User{SourceID: 7, Kind: "plex", ExternalID: "stable-person"}
	renumbered := User{SourceID: 99, Kind: "plex", ExternalID: "stable-person"}
	reusedNumeric := User{SourceID: 7, Kind: "plex", ExternalID: "different-person"}
	if stablePersonKey(Seerr, first) != stablePersonKey(Seerr, renumbered) {
		t.Fatal("the same provider identity changed when its numeric source id changed")
	}
	if stablePersonKey(Seerr, first) == stablePersonKey(Seerr, reusedNumeric) {
		t.Fatal("a reused numeric source id collapsed different provider identities")
	}
}

func TestCompatibilityNumericPersonIdentityIsOnlyReusableWithinOneMove(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	newMove := func() int64 {
		var id int64
		if err := l.st.Write(ctx, func(tx *sql.Tx) error {
			res, err := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,'running')", database.FormatTime(time.Now().UTC()))
			if err == nil {
				id, err = res.LastInsertId()
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	it := Item{Key: "person:seerr:77", Name: "First numeric identity", Person: &PersonPlan{App: "seerr", SourceID: 77, Role: "member", Perms: []string{}}}
	r1 := &run{m: l.mv, ctx: ctx, move: newMove(), by: domain.Actor{Kind: "user", ID: 1}, owner: 1, people: map[string]int64{}, rep: &Report{Counts: map[string]int{}}}
	if got, why := r1.one(GroupPeople, it); got != ResCreated {
		t.Fatalf("first numeric identity %s: %s", got, why)
	}
	created := r1.people[it.Key]
	if got, _ := r1.one(GroupPeople, it); got != ResAlready || r1.people[it.Key] != created {
		t.Fatalf("same move did not reuse its numeric identity: %s %+v", got, r1.people)
	}
	r2 := &run{m: l.mv, ctx: ctx, move: newMove(), by: domain.Actor{Kind: "user", ID: 1}, owner: 1, people: map[string]int64{}, rep: &Report{Counts: map[string]int{}}}
	reused := it
	reused.Name = "Different person after source id reuse"
	if got, why := r2.one(GroupPeople, reused); got != ResSkipped || r2.people[it.Key] != -2 || r2.choose != ChooseNumericID {
		t.Fatalf("later numeric identity was silently reused: %s %q %+v", got, why, r2.people)
	}
	// After a choice the line still says why, so the screen keeps its row.
	left := reused
	left.Person = &PersonPlan{App: "seerr", SourceID: 77, Role: "member", Perms: []string{}, Choice: PersonSkip}
	r3 := &run{m: l.mv, ctx: ctx, move: newMove(), by: domain.Actor{Kind: "user", ID: 1}, owner: 1, people: map[string]int64{}, rep: &Report{Counts: map[string]int{}}}
	if got, why := r3.one(GroupPeople, left); got != ResSkipped || r3.choose != ChooseNumericID {
		t.Fatalf("a made choice lost its row: %s %q %q", got, why, r3.choose)
	}
	if n := l.count(t, "SELECT count(*) FROM users WHERE id<>1"); n != 1 {
		t.Fatalf("numeric source-id reuse created or rebound %d users", n)
	}
}

func TestCompatibilitySkippedPeopleCannotOwnOrDecideLaterMoveRequest(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	newMove := func() int64 {
		var id int64
		if err := l.st.Write(ctx, func(tx *sql.Tx) error {
			res, err := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,'running')", database.FormatTime(time.Now().UTC()))
			if err == nil {
				id, err = res.LastInsertId()
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	asker := Item{Key: "person:seerr:78", Name: "First numeric asker", Person: &PersonPlan{App: "seerr", SourceID: 78, Role: "member", Perms: []string{}}}
	decider := Item{Key: "person:seerr:79", Name: "First numeric decider", Person: &PersonPlan{App: "seerr", SourceID: 79, Role: "member", Perms: []string{}}}
	first := &run{m: l.mv, ctx: ctx, move: newMove(), by: domain.Actor{Kind: "user", ID: 1}, owner: 1, people: map[string]int64{}, rep: &Report{Counts: map[string]int{}}}
	for _, person := range []Item{asker, decider} {
		if got, why := first.one(GroupPeople, person); got != ResCreated {
			t.Fatalf("first numeric identity %s: %s", got, why)
		}
	}
	oldAsker, oldDecider := first.people[asker.Key], first.people[decider.Key]
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		for _, tmdb := range []string{"700078", "700079", "700080"} {
			if _, err := database.Insert(tx, "titles", &mediafiles.Title{Kind: "movie", Name: "Numeric guard " + tmdb, SortName: "numeric guard " + tmdb,
				Aliases: []string{}, SourceIDs: map[string]string{"tmdb": tmdb}, MetadataSource: "tmdb", Artwork: []string{}, State: "active",
				Status: "released", ReleaseGate: "released", GateDriver: "none", NewPartsPolicy: "all", WhyAdded: "migration"}, domain.System); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	runCase := func(name, ref string, peopleSkipped, explicitSkip bool) {
		t.Helper()
		people := []Item{asker, decider}
		if explicitSkip {
			for i := range people {
				copyPlan := *people[i].Person
				copyPlan.Choice = PersonSkip
				people[i].Person = &copyPlan
			}
		}
		p := &Plan{Apps: []string{"seerr"}, Groups: []Group{
			{Name: GroupPeople, Label: GroupLabels[GroupPeople], Skipped: peopleSkipped, Items: people},
			{Name: GroupRequests, Label: GroupLabels[GroupRequests], Items: []Item{{
				Key: "ask:seerr:" + name, Name: "Numeric reuse guard " + name,
				Ask: &AskPlan{App: "seerr", Kind: "movie", Ref: "tmdb:" + ref, IDs: map[string]string{"tmdb": ref}, By: asker.Key, Approved: true, Decider: decider.Key},
			}}},
		}}
		rep := l.mv.Run(ctx, newMove(), p, false, domain.Actor{Kind: "user", ID: 1}, nil)
		if rep.Counts[ResCreated] != 1 || rep.Counts[ResSkipped] != 2 {
			t.Fatalf("%s unexpected full-run results: %+v", name, rep.Counts)
		}
		if n := l.count(t, `SELECT count(*) FROM asks a JOIN ask_people p ON p.ask_id=a.id
			WHERE a.ref=? AND a.state='approved' AND a.asked_by=1 AND a.decided_by=1 AND p.user_id=1`, "tmdb:"+ref); n != 1 {
			t.Fatalf("%s did not fall back to the owner for request and decision ownership", name)
		}
		if n := l.count(t, `SELECT count(*) FROM asks a JOIN ask_people p ON p.ask_id=a.id
			WHERE a.ref=? AND (a.asked_by IN (?,?) OR a.decided_by IN (?,?) OR p.user_id IN (?,?))`,
			"tmdb:"+ref, oldAsker, oldDecider, oldAsker, oldDecider, oldAsker, oldDecider); n != 0 {
			t.Fatalf("%s silently reused a numeric identity from an earlier move", name)
		}
	}
	runCase("group-skipped", "700078", true, false)
	runCase("people-explicitly-skipped", "700079", false, true)

	stableAsker := Item{Key: "person:seerr:plex:stable-asker", Name: "Stable asker", Person: &PersonPlan{App: "seerr", SourceID: 80, StableIdentity: true, Role: "member", Perms: []string{}}}
	stableDecider := Item{Key: "person:seerr:plex:stable-decider", Name: "Stable decider", Person: &PersonPlan{App: "seerr", SourceID: 81, StableIdentity: true, Role: "member", Perms: []string{}}}
	for _, person := range []Item{stableAsker, stableDecider} {
		if got, why := first.one(GroupPeople, person); got != ResCreated {
			t.Fatalf("first stable identity %s: %s", got, why)
		}
	}
	oldStableAsker, oldStableDecider := first.people[stableAsker.Key], first.people[stableDecider.Key]
	stablePeople := []Item{stableAsker, stableDecider}
	for i := range stablePeople {
		copyPlan := *stablePeople[i].Person
		copyPlan.Choice = PersonSkip
		stablePeople[i].Person = &copyPlan
	}
	stablePlan := &Plan{Apps: []string{"seerr"}, Groups: []Group{
		{Name: GroupPeople, Label: GroupLabels[GroupPeople], Items: stablePeople},
		{Name: GroupRequests, Label: GroupLabels[GroupRequests], Items: []Item{{
			Key: "ask:seerr:stable-explicit-skip", Name: "Stable explicit skip guard",
			Ask: &AskPlan{App: "seerr", Kind: "movie", Ref: "tmdb:700080", IDs: map[string]string{"tmdb": "700080"}, By: stableAsker.Key, Approved: true, Decider: stableDecider.Key},
		}}},
	}}
	rep := l.mv.Run(ctx, newMove(), stablePlan, false, domain.Actor{Kind: "user", ID: 1}, nil)
	if rep.Counts[ResCreated] != 1 || rep.Counts[ResSkipped] != 2 {
		t.Fatalf("stable explicit skip unexpected full-run results: %+v", rep.Counts)
	}
	if n := l.count(t, `SELECT count(*) FROM asks a JOIN ask_people p ON p.ask_id=a.id
		WHERE a.ref='tmdb:700080' AND a.state='approved' AND a.asked_by=1 AND a.decided_by=1 AND p.user_id=1`); n != 1 {
		t.Fatal("stable explicit skip did not fall back to the owner for request and decision ownership")
	}
	if n := l.count(t, `SELECT count(*) FROM asks a JOIN ask_people p ON p.ask_id=a.id
		WHERE a.ref='tmdb:700080' AND (a.asked_by IN (?,?) OR a.decided_by IN (?,?) OR p.user_id IN (?,?))`,
		oldStableAsker, oldStableDecider, oldStableAsker, oldStableDecider, oldStableAsker, oldStableDecider); n != 0 {
		t.Fatal("stable explicit skip silently reused a person from an earlier move")
	}
}

func TestCompatibilityPersonChoicePrecedesPriorIdentityMarks(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	newMove := func() int64 {
		var id int64
		if err := l.st.Write(ctx, func(tx *sql.Tx) error {
			res, err := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,'running')", database.FormatTime(time.Now().UTC()))
			if err == nil {
				id, err = res.LastInsertId()
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	type peoplePair struct {
		items  []Item
		oldIDs []int64
	}
	first := &run{m: l.mv, ctx: ctx, move: newMove(), by: domain.Actor{Kind: "user", ID: 1}, owner: 1, people: map[string]int64{}, rep: &Report{Counts: map[string]int{}}}
	makePair := func(prefix string, base int64, stable bool) peoplePair {
		t.Helper()
		keys := []string{"person:seerr:" + strconv.FormatInt(base, 10), "person:seerr:" + strconv.FormatInt(base+1, 10)}
		if stable {
			keys = []string{"person:seerr:plex:" + prefix + "-asker", "person:seerr:plex:" + prefix + "-decider"}
		}
		pair := peoplePair{items: []Item{
			{Key: keys[0], Name: prefix + " old asker", Person: &PersonPlan{App: "seerr", SourceID: base, StableIdentity: stable, Role: "member", Perms: []string{}}},
			{Key: keys[1], Name: prefix + " old decider", Person: &PersonPlan{App: "seerr", SourceID: base + 1, StableIdentity: stable, Role: "member", Perms: []string{}}},
		}}
		for _, person := range pair.items {
			if got, why := first.one(GroupPeople, person); got != ResCreated {
				t.Fatalf("create %s: %s", got, why)
			}
			pair.oldIDs = append(pair.oldIDs, first.people[person.Key])
		}
		return pair
	}
	numeric := makePair("numeric", 90, false)
	stable := makePair("stable", 92, true)
	mergeIDs := make([]int64, 2)
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		for i, name := range []string{"Chosen merge asker", "Chosen merge decider"} {
			id, err := database.Insert(tx, "users", &domain.User{Name: name, Role: "member", PasswordHash: "", State: "disabled", Perms: "[]"}, domain.System)
			if err != nil {
				return err
			}
			mergeIDs[i] = id
			if _, err := tx.Exec("INSERT INTO user_ask_settings (user_id) VALUES (?)", id); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	addMovie := func(ref string) {
		t.Helper()
		if err := l.st.Write(ctx, func(tx *sql.Tx) error {
			_, err := database.Insert(tx, "titles", &mediafiles.Title{Kind: "movie", Name: "Choice guard " + ref, SortName: "choice guard " + ref,
				Aliases: []string{}, SourceIDs: map[string]string{"tmdb": ref}, MetadataSource: "tmdb", Artwork: []string{}, State: "active",
				Status: "released", ReleaseGate: "released", GateDriver: "none", NewPartsPolicy: "all", WhyAdded: "migration"}, domain.System)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	type choiceCase struct {
		name         string
		pair         peoplePair
		groupSkipped bool
		choice       string
		role         string
		merge        bool
		wantAsk      bool
		wantIDs      []int64
	}
	cases := []choiceCase{
		{name: "numeric-group-skip", pair: numeric, groupSkipped: true, wantAsk: true, wantIDs: []int64{1, 1}},
		{name: "stable-group-skip", pair: stable, groupSkipped: true, wantAsk: true, wantIDs: []int64{1, 1}},
		{name: "numeric-person-skip", pair: numeric, choice: PersonSkip, wantAsk: true, wantIDs: []int64{1, 1}},
		{name: "stable-person-skip", pair: stable, choice: PersonSkip, wantAsk: true, wantIDs: []int64{1, 1}},
		{name: "numeric-owner", pair: numeric, choice: PersonOwner, role: "owner", wantAsk: true, wantIDs: []int64{1, 1}},
		{name: "stable-owner", pair: stable, choice: PersonOwner, role: "owner", wantAsk: true, wantIDs: []int64{1, 1}},
		{name: "numeric-merge", pair: numeric, choice: PersonMerge, merge: true, wantAsk: true, wantIDs: mergeIDs},
		{name: "stable-merge", pair: stable, choice: PersonMerge, merge: true, wantAsk: true, wantIDs: mergeIDs},
		{name: "numeric-default", pair: numeric, wantAsk: false},
		{name: "stable-default", pair: stable, wantAsk: true, wantIDs: stable.oldIDs},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ref := strconv.Itoa(710000 + i)
			addMovie(ref)
			people := make([]Item, len(tc.pair.items))
			for j := range tc.pair.items {
				people[j] = tc.pair.items[j]
				copyPlan := *people[j].Person
				copyPlan.Choice = tc.choice
				if tc.role != "" {
					copyPlan.Role = tc.role
				}
				people[j].Person = &copyPlan
				if tc.merge {
					people[j].Name = []string{"Chosen merge asker", "Chosen merge decider"}[j]
				}
			}
			p := &Plan{Apps: []string{"seerr"}, Groups: []Group{
				{Name: GroupPeople, Label: GroupLabels[GroupPeople], Skipped: tc.groupSkipped, Items: people},
				{Name: GroupRequests, Label: GroupLabels[GroupRequests], Items: []Item{{Key: "ask:seerr:" + tc.name, Name: tc.name,
					Ask: &AskPlan{App: "seerr", Kind: "movie", Ref: "tmdb:" + ref, IDs: map[string]string{"tmdb": ref}, By: people[0].Key, Approved: true, Decider: people[1].Key}}}},
			}}
			moveID := newMove()
			rep := l.mv.Run(ctx, moveID, p, false, domain.Actor{Kind: "user", ID: 1}, nil)
			if rep.Counts[ResFailed] != 0 {
				t.Fatalf("failed result: %+v", rep.Lines)
			}
			if !tc.wantAsk {
				if n := l.count(t, "SELECT count(*) FROM asks WHERE ref=?", "tmdb:"+ref); n != 0 {
					t.Fatal("unverifiable numeric default created a request")
				}
				return
			}
			if n := l.count(t, `SELECT count(*) FROM asks a JOIN ask_people p ON p.ask_id=a.id
				WHERE a.ref=? AND a.asked_by=? AND a.decided_by=? AND p.user_id=?`, "tmdb:"+ref, tc.wantIDs[0], tc.wantIDs[1], tc.wantIDs[0]); n != 1 {
				t.Fatalf("choice did not control request identities: want %+v", tc.wantIDs)
			}
			if tc.name == "stable-default" {
				repeat := l.mv.Run(ctx, moveID, p, false, domain.Actor{Kind: "user", ID: 1}, nil)
				if repeat.Counts[ResAlready] != 3 || l.count(t, "SELECT count(*) FROM asks WHERE ref=?", "tmdb:"+ref) != 1 {
					t.Fatalf("same-move stable identity repeat was not idempotent: %+v", repeat.Counts)
				}
			}
		})
	}
	if n := l.count(t, "SELECT count(*) FROM users WHERE role='owner'"); n != 1 {
		t.Fatalf("person choices elevated %d users to owner", n)
	}
	if n := l.count(t, "SELECT count(*) FROM users WHERE role='admin'"); n != 0 {
		t.Fatalf("person choices elevated %d users to admin", n)
	}
}

func TestCompatibilityQueuedMoveRequiresExactServerApprovedPlan(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	choices := Choices{PathRules: l.rules, People: l.people, ApprovedHash: "caller supplied"}
	p, err := LoadPlan(ctx, l.st, l.snaps, choices)
	if err != nil {
		t.Fatal(err)
	}
	mv, err := l.mv.Start(ctx, l.snaps, choices, PlanHash(p), domain.Actor{Kind: "user", ID: 1})
	if err != nil {
		t.Fatal(err)
	}
	kept, err := GetMove(ctx, l.st, mv.ID)
	if err != nil || kept.Choices.ApprovedHash != PlanHash(p) {
		t.Fatalf("server approval was not persisted: %q %v", kept.Choices.ApprovedHash, err)
	}
	var raw string
	if err := l.st.Read().QueryRow("SELECT data FROM migrate_snapshots WHERE id=?", l.snaps[0]).Scan(&raw); err != nil {
		t.Fatal(err)
	}
	var changed Snapshot
	if err := json.Unmarshal([]byte(raw), &changed); err != nil {
		t.Fatal(err)
	}
	changed.Notifications = append(changed.Notifications, Provider{Name: "changed after approval", Kind: "Discord", Enabled: true})
	b, _ := json.Marshal(changed)
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE migrate_snapshots SET data=?, size=? WHERE id=?", string(b), len(b), l.snaps[0])
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := l.q.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer l.q.Stop()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		got, err := GetMove(ctx, l.st, mv.ID)
		if err != nil {
			t.Fatal(err)
		}
		if got.State == "failed" {
			if n := l.count(t, "SELECT count(*) FROM move_made WHERE move_id=?", mv.ID); n != 0 {
				t.Fatalf("changed plan wrote %d move marks", n)
			}
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("changed queued plan did not fail closed")
}

func TestCompatibilityLegacyQueuedMoveWithoutApprovalFailsClosed(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	var moveID int64
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		snaps, _ := json.Marshal(l.snaps)
		choices, _ := json.Marshal(Choices{PathRules: l.rules, People: l.people})
		res, err := tx.Exec("INSERT INTO moves (created_at, owner_id, snapshots, choices, state) VALUES (?,1,?,?,'queued')", database.FormatTime(time.Now().UTC()), string(snaps), string(choices))
		if err == nil {
			moveID, err = res.LastInsertId()
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	// Directly exercise the recovered job boundary. It must reject the old
	// row before Run can write any imported domain row.
	err := l.mv.handle(ctx, &jobs.Run{Job: domain.Job{Subject: &domain.Subject{Type: "move", ID: moveID}, CreatedBy: domain.Actor{Kind: "user", ID: 1}}})
	if !errors.Is(err, ErrPlanChanged) {
		t.Fatalf("legacy approval error = %v", err)
	}
	if n := l.count(t, "SELECT count(*) FROM move_made WHERE move_id=?", moveID); n != 0 {
		t.Fatalf("legacy unapproved move wrote %d marks", n)
	}
}

func TestCompatibilityRecoveryKeepsExactApprovalAndRejectsTamperedHash(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	choices := Choices{PathRules: l.rules, People: l.people}
	plan, err := LoadPlan(ctx, l.st, l.snaps, choices)
	if err != nil {
		t.Fatal(err)
	}
	mv, err := l.mv.Start(ctx, l.snaps, choices, PlanHash(plan), domain.Actor{Kind: "user", ID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := l.mv.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := GetMove(ctx, l.st, mv.ID)
	if err != nil || got.State != "queued" {
		t.Fatalf("unchanged approved move was not recoverable: %+v %v", got, err)
	}
	got.Choices.ApprovedHash = strings.Repeat("0", 64)
	raw, _ := json.Marshal(got.Choices)
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE moves SET choices=? WHERE id=?", string(raw), mv.ID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := l.mv.Recover(ctx); err != nil {
		t.Fatal(err)
	}
	got, err = GetMove(ctx, l.st, mv.ID)
	if err != nil || got.State != "failed" {
		t.Fatalf("tampered approval was not failed closed: %+v %v", got, err)
	}
	if n := l.count(t, "SELECT count(*) FROM move_made WHERE move_id=?", mv.ID); n != 0 {
		t.Fatal("tampered recovered move changed domain data")
	}
}

func TestCompatibilityFailedSupervisedUndoCanBeRetried(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	var moveID, userID int64
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,'done')", database.FormatTime(time.Now().UTC()))
		if err != nil {
			return err
		}
		moveID, _ = res.LastInsertId()
		userID, err = database.Insert(tx, "users", &domain.User{Name: "undo retry", Role: "member", State: "disabled", Perms: "[]"}, domain.System)
		if err != nil {
			return err
		}
		r := &run{m: l.mv, ctx: ctx, move: moveID}
		if err := r.markTx(tx, "person:seerr:retry", "user", userID, "", ""); err != nil {
			return err
		}
		_, err = tx.Exec("CREATE TRIGGER fail_move_user_undo BEFORE DELETE ON users WHEN OLD.id = " + strconv.FormatInt(userID, 10) + " BEGIN SELECT RAISE(ABORT, 'temporary undo failure'); END")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	first := l.undo(t, moveID)
	if first.State != "done" || first.UndoState != "failed" || first.Report == nil || first.Report.Counts[ResFailed] == 0 {
		t.Fatalf("failed undo was finalized or lost its report: %+v", first)
	}
	if n := l.count(t, "SELECT count(*) FROM users WHERE id=?", userID); n != 1 {
		t.Fatal("failed undo changed its protected row")
	}
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("DROP TRIGGER fail_move_user_undo")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	second := l.undo(t, moveID)
	if second.State != "undone" || second.UndoState != "" {
		t.Fatalf("retry did not finish undo: %+v", second)
	}
	if n := l.count(t, "SELECT count(*) FROM users WHERE id=?", userID); n != 0 {
		t.Fatal("retry did not remove the moved row")
	}
}

func TestCompatibilityPlanKeepsUnlimitedWishlistDefaults(t *testing.T) {
	seerr := &Snapshot{Kind: Seerr, Settings: map[string]any{
		"default_movie_limit": int64(0), "default_movie_days": int64(7),
		"default_show_limit": int64(0), "default_show_days": int64(7),
	}}
	p := BuildPlan([]Input{{ID: 1, Snap: seerr}}, Choices{})
	defaults := item(t, p, GroupRequests, "wishlist_defaults")
	if defaults.AskSettings == nil || defaults.AskSettings.MoviesPerWeek == nil || *defaults.AskSettings.MoviesPerWeek != 0 ||
		defaults.AskSettings.ShowsPerWeek == nil || *defaults.AskSettings.ShowsPerWeek != 0 {
		t.Fatalf("unlimited defaults not retained: %+v", defaults.AskSettings)
	}
}

func TestCompatibilityDefaultQuotaAccountingRejectsOrphansAndPairs(t *testing.T) {
	orphan := BuildPlan([]Input{{ID: 1, Snap: &Snapshot{Kind: Seerr, Settings: map[string]any{"default_movie_days": int64(7)}}}}, Choices{})
	if _, ok := notMoved(orphan, "seerr settings.default_movie_days"); !ok {
		t.Fatal("orphan default quota days were declared mapped")
	}
	rejected := BuildPlan([]Input{{ID: 2, Snap: &Snapshot{Kind: Seerr, Settings: map[string]any{"default_show_limit": int64(1001), "default_show_days": int64(7)}}}}, Choices{})
	for _, from := range []string{"seerr settings.default_show_limit", "seerr settings.default_show_days"} {
		if _, ok := notMoved(rejected, from); !ok {
			t.Fatalf("rejected default quota field was not explicit: %s", from)
		}
	}
}

func TestCompatibilityConfirmedAdminDoesNotSilentlyWidenOrDropCapabilities(t *testing.T) {
	s := &Snapshot{Kind: Seerr, Users: []User{{SourceID: 8, Name: "Admin", Admin: true, Permissions: []string{"ADMIN"}}}}
	p := BuildPlan([]Input{{ID: 1, Snap: s}}, Choices{People: map[string]string{"person:seerr:8": PersonAdmin}})
	it := item(t, p, GroupPeople, "person:seerr:8")
	if it.Person.Role != "admin" || !slices.Equal(it.Person.Perms, []string{"ask"}) {
		t.Fatalf("confirmed admin mapping widened: %+v", it.Person)
	}
	if n, ok := notMoved(p, "seerr users[0].permissions"); !ok || !strings.Contains(n.What, "ADMIN") {
		t.Fatalf("source-admin capability loss not reported: %+v", n)
	}
}

func TestCompatibilityLegacySnapshotNegativeQuotasRemainExplicit(t *testing.T) {
	s := &Snapshot{Kind: Seerr, Users: []User{{SourceID: 9, Name: "Legacy", MovieLimit: -1, MovieDays: 7, ShowLimit: -2, ShowDays: 7}}}
	p := BuildPlan([]Input{{ID: 1, Snap: s}}, Choices{})
	for _, from := range []string{"seerr users[0].movie_quota", "seerr users[0].show_quota"} {
		if _, ok := notMoved(p, from); !ok {
			t.Fatalf("legacy out-of-range quota disappeared: %s", from)
		}
	}
}

func TestCompatibilityAppliesArtistAlbumAndPressingPolicy(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	var titleID, partID, oldPressing, wantedPressing int64
	err := l.st.Write(ctx, func(tx *sql.Tx) error {
		var err error
		titleID, err = database.Insert(tx, "titles", &mediafiles.Title{Kind: "artist", Name: "Artist", SortName: "artist", Aliases: []string{},
			SourceIDs: map[string]string{"musicbrainz": "056e4f3e-d505-4dad-8ec1-d04f521cbb56"}, MetadataSource: "musicbrainz",
			KeepUpToDate: true, NewPartsPolicy: "all", ReleaseGate: "released", MusicTypes: []string{"album", "ep"}, WhyAdded: "migration",
			Artwork: []string{}, Status: "running", State: "active", GateDriver: "none", MusicSecondary: []string{"studio"}, MusicStatuses: []string{"official"}}, domain.System)
		if err != nil {
			return err
		}
		partID, err = database.Insert(tx, "parts", &mediafiles.Part{TitleID: titleID, Kind: "album", Name: "Album", KeepUpToDate: true,
			SourceIDs: map[string]string{"musicbrainz": "11111111-1111-1111-1111-111111111111"}, Pressings: []mediafiles.Pressing{}, SecondaryTypes: []string{}, Included: true}, domain.System)
		if err != nil {
			return err
		}
		if oldPressing, err = database.Insert(tx, "pressings", &domain.AlbumPressing{PartID: partID, MBID: "33333333-3333-3333-3333-333333333333", Name: "Old", Formats: []string{}, Tracks: []domain.PressingTrack{}, Chosen: true, ChosenBy: "auto"}, domain.System); err != nil {
			return err
		}
		wantedPressing, err = database.Insert(tx, "pressings", &domain.AlbumPressing{PartID: partID, MBID: "22222222-2222-2222-2222-222222222222", Name: "Wanted", Formats: []string{}, Tracks: []domain.PressingTrack{}, ChosenBy: "auto"}, domain.System)
		if err != nil {
			return err
		}
		_, err = database.Insert(tx, "items", &mediafiles.Item{TitleID: titleID, PartID: &partID, Kind: "track", Numbers: mediafiles.Numbers{}, KeepUpToDate: true}, domain.System)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	var moveID int64
	err = l.st.Write(ctx, func(tx *sql.Tx) error {
		res, e := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,'running')", database.FormatTime(time.Now().UTC()))
		if e == nil {
			moveID, e = res.LastInsertId()
		}
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	future := "future_only"
	r := &run{m: l.mv, ctx: ctx, by: domain.Actor{Kind: "user", ID: 1}, move: moveID}
	res, detail := r.title(Item{Key: "title:artist:test", Title: &TitlePlan{Kind: "artist", ID: "mbid:056e4f3e-d505-4dad-8ec1-d04f521cbb56", KeepUpToDate: false, Music: &MusicPolicyPlan{NewAlbums: &future, AlbumTypes: []string{"album"}, SecondaryTypes: []string{"studio", "live"}, ReleaseStatuses: []string{"official"}, Albums: []MusicAlbumPlan{{MBID: "11111111-1111-1111-1111-111111111111", KeepUpToDate: false, PressingMBID: "22222222-2222-2222-2222-222222222222"}}}}})
	if res != ResChanged {
		t.Fatalf("existing artist policy: %s %s", res, detail)
	}
	if n := l.count(t, "SELECT count(*) FROM titles WHERE id = ? AND keep_up_to_date = 0 AND new_parts_policy = 'future_only' AND music_types = '[\"album\"]' AND music_secondary = '[\"studio\",\"live\"]' AND music_statuses = '[\"official\"]'", titleID); n != 1 {
		t.Fatal("artist policy not applied")
	}
	if n := l.count(t, "SELECT count(*) FROM parts WHERE id = ? AND keep_up_to_date = 0", partID); n != 1 {
		t.Fatal("album monitoring not applied")
	}
	if n := l.count(t, "SELECT count(*) FROM pressings WHERE id = ? AND chosen = 1 AND chosen_by = 'person'", wantedPressing); n != 1 {
		t.Fatal("pressing identity not applied")
	}
	if n := l.count(t, "SELECT count(*) FROM items WHERE part_id = ? AND keep_up_to_date = 0", partID); n != 1 {
		t.Fatal("track monitoring did not follow album")
	}
	var mark madeRow
	if err := l.st.Read().QueryRow("SELECT key, what, row_id, before, after, fingerprint FROM move_made WHERE move_id=?", moveID).Scan(&mark.key, &mark.what, &mark.row, &mark.before, &mark.after, &mark.fp); err != nil {
		t.Fatal(err)
	}
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, e := tx.Exec("UPDATE pressings SET dropped_at=? WHERE id=?", database.FormatTime(time.Now().UTC()), oldPressing)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := l.mv.undoOne(ctx, mark, domain.Actor{Kind: "user", ID: 1}); got != ResFailed {
		t.Fatalf("dropped prior pressing undo %s", got)
	}
	if n := l.count(t, "SELECT count(*) FROM pressings WHERE id=? AND chosen=1", wantedPressing); n != 1 {
		t.Fatal("failed undo changed the current pressing")
	}
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, e := tx.Exec("UPDATE pressings SET dropped_at=NULL WHERE id=?", oldPressing)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	var partCreated, itemCreated string
	if err := l.st.Read().QueryRow("SELECT created_at FROM parts WHERE id=?", partID).Scan(&partCreated); err != nil {
		t.Fatal(err)
	}
	if err := l.st.Read().QueryRow("SELECT created_at FROM items WHERE part_id=? LIMIT 1", partID).Scan(&itemCreated); err != nil {
		t.Fatal(err)
	}
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE parts SET created_at=? WHERE id=?", "2099-01-01T00:00:00Z", partID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := l.mv.undoOne(ctx, mark, domain.Actor{Kind: "user", ID: 1}); got != ResKept {
		t.Fatalf("replaced album row was changed by undo: %s", got)
	}
	if n := l.count(t, "SELECT count(*) FROM pressings WHERE id=? AND chosen=1", wantedPressing); n != 1 {
		t.Fatal("replaced album guard changed current pressing")
	}
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE parts SET created_at=? WHERE id=?", partCreated, partID); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE items SET created_at=? WHERE part_id=?", "2099-01-01T00:00:00Z", partID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := l.mv.undoOne(ctx, mark, domain.Actor{Kind: "user", ID: 1}); got != ResKept {
		t.Fatalf("replaced track row was changed by undo: %s", got)
	}
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE items SET created_at=? WHERE part_id=?", itemCreated, partID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := l.mv.undoOne(ctx, mark, domain.Actor{Kind: "user", ID: 1}); got != ResChanged {
		t.Fatalf("policy undo %s", got)
	}
	if n := l.count(t, "SELECT count(*) FROM titles WHERE id=? AND keep_up_to_date=1 AND new_parts_policy='all' AND music_types='[\"album\",\"ep\"]'", titleID); n != 1 {
		t.Fatal("artist policy not restored")
	}
	if n := l.count(t, "SELECT count(*) FROM pressings WHERE part_id=? AND mbid='33333333-3333-3333-3333-333333333333' AND chosen=1 AND chosen_by='auto'", partID); n != 1 {
		t.Fatal("pressing choice provenance not restored")
	}
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, e := tx.Exec("UPDATE pressings SET chosen=0 WHERE part_id=?", partID)
		return e
	}); err != nil {
		t.Fatal(err)
	}
	err = l.st.Write(ctx, func(tx *sql.Tx) error {
		res, e := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,'running')", database.FormatTime(time.Now().UTC()))
		if e == nil {
			moveID, e = res.LastInsertId()
		}
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	r.move = moveID
	if got, why := r.title(Item{Key: "title:artist:no-prior-choice", Title: &TitlePlan{Kind: "artist", ID: "mbid:056e4f3e-d505-4dad-8ec1-d04f521cbb56", Music: &MusicPolicyPlan{Albums: []MusicAlbumPlan{{MBID: "11111111-1111-1111-1111-111111111111", KeepUpToDate: true, PressingMBID: "22222222-2222-2222-2222-222222222222"}}}}}); got != ResChanged {
		t.Fatalf("second policy %s %s", got, why)
	}
	if err := l.st.Read().QueryRow("SELECT key, what, row_id, before, after, fingerprint FROM move_made WHERE move_id=?", moveID).Scan(&mark.key, &mark.what, &mark.row, &mark.before, &mark.after, &mark.fp); err != nil {
		t.Fatal(err)
	}
	if got, _ := l.mv.undoOne(ctx, mark, domain.Actor{Kind: "user", ID: 1}); got != ResChanged {
		t.Fatalf("second policy undo %s", got)
	}
	if n := l.count(t, "SELECT count(*) FROM pressings WHERE part_id=? AND chosen=1", partID); n != 0 {
		t.Fatal("undo invented a pressing choice")
	}
}

func TestCompatibilityPlanKeepsSupportedLidarrPolicy(t *testing.T) {
	lidarr := &Snapshot{Kind: Lidarr,
		Folders: []Folder{{Path: "/music"}},
		MetadataProfiles: []Named{{ID: 2, Name: "Standard", Settings: map[string]any{
			"primaryalbumtypes":   []any{map[string]any{"name": "Album", "allowed": true}, map[string]any{"name": "Single", "allowed": false}},
			"secondaryalbumtypes": []any{map[string]any{"name": "Studio", "allowed": true}, map[string]any{"name": "Live", "allowed": true}},
			"releasestatuses":     []any{map[string]any{"name": "Official", "allowed": true}, map[string]any{"name": "Bootleg", "allowed": false}},
		}}},
		Titles: []Title{{SourceID: 7, Kind: "artist", Name: "Artist", IDs: map[string]string{"musicbrainz": "056e4f3e-d505-4dad-8ec1-d04f521cbb56"},
			Path: "/music/Artist", Monitored: true, Extra: map[string]any{"new_albums": "new", "metadata_profile_id": int64(2)},
			MusicAlbums: []MusicAlbum{{MBID: "11111111-1111-1111-1111-111111111111", Monitored: false, WatchedEdition: "22222222-2222-2222-2222-222222222222"}}}},
	}
	p := BuildPlan([]Input{{ID: 3, Snap: lidarr}}, Choices{})
	it := item(t, p, GroupTitles, "title:artist:mbid:056e4f3e-d505-4dad-8ec1-d04f521cbb56")
	future := "future_only"
	want := &MusicPolicyPlan{NewAlbums: &future, AlbumTypes: []string{"album"}, SecondaryTypes: []string{"studio", "live"}, ReleaseStatuses: []string{"official"},
		Albums: []MusicAlbumPlan{{MBID: "11111111-1111-1111-1111-111111111111", KeepUpToDate: false, PressingMBID: "22222222-2222-2222-2222-222222222222"}}}
	if it.Title.Music == nil || it.Title.Music.NewAlbums == nil || *it.Title.Music.NewAlbums != *want.NewAlbums || !slices.Equal(it.Title.Music.AlbumTypes, want.AlbumTypes) ||
		!slices.Equal(it.Title.Music.SecondaryTypes, want.SecondaryTypes) || !slices.Equal(it.Title.Music.ReleaseStatuses, want.ReleaseStatuses) ||
		!slices.Equal(it.Title.Music.Albums, want.Albums) {
		t.Fatalf("music policy not retained: %+v", it.Title.Music)
	}
}

func TestCompatibilityApplyDefaultsLimitsRelationshipsRepeatAndUndo(t *testing.T) {
	l := newMoveLabWith(t, func(_, _, seerr *Snapshot) {
		seerr.Settings = map[string]any{"default_movie_limit": int64(4), "default_movie_days": int64(7), "default_show_limit": int64(2), "default_show_days": int64(7)}
		seerr.Users[1].ShowLimit, seerr.Users[1].ShowDays = 2, 7
		seerr.Requests = append(seerr.Requests, Request{SourceID: 8, Kind: "movie", IDs: map[string]string{"tmdb": "604"}, State: "pending", By: 1, ByName: "luis", Created: "2026-09-02T10:00:00Z"})
	})
	beforeBytes, err := os.ReadFile(l.file)
	if err != nil {
		t.Fatal(err)
	}
	beforeInfo, err := os.Stat(l.file)
	if err != nil {
		t.Fatal(err)
	}
	beforeHash := sha256.Sum256(beforeBytes)
	preview := l.mv.Run(context.Background(), 0, l.plan(t), true, domain.Actor{Kind: "user", ID: 1}, nil)
	if preview.Counts[ResFailed] != 0 || preview.Counts[ResChange] == 0 {
		t.Fatalf("preview did not show a safe settings change: %+v", preview.Counts)
	}
	if n := l.count(t, "SELECT count(*) FROM settings WHERE grp = 'this_app' AND key IN ('ask_movies_per_week','ask_shows_per_week')"); n != 0 {
		t.Fatalf("preview wrote settings: %d", n)
	}
	mv := l.move(t, Choices{})
	if mv.State != "done" {
		t.Fatalf("move state %s", mv.State)
	}
	if n := l.count(t, "SELECT count(*) FROM settings WHERE grp = 'this_app' AND ((key = 'ask_movies_per_week' AND value = '4') OR (key = 'ask_shows_per_week' AND value = '2'))"); n != 2 {
		t.Fatalf("default settings count %d", n)
	}
	var ana int64
	_ = l.st.Read().QueryRow("SELECT id FROM users WHERE name = 'Ana'").Scan(&ana)
	if n := l.count(t, "SELECT count(*) FROM user_ask_settings WHERE user_id = ? AND movies_per_week = 3 AND shows_per_week = 2 AND per_week IS NULL", ana); n != 1 {
		t.Fatalf("per-kind user limit count %d", n)
	}
	if n := l.count(t, "SELECT count(*) FROM asks WHERE ref = 'tmdb:604'"); n != 1 {
		t.Fatalf("deduplicated asks %d", n)
	}
	if n := l.count(t, "SELECT count(*) FROM ask_people p JOIN asks a ON a.id = p.ask_id WHERE a.ref = 'tmdb:604' AND p.counted = 0"); n != 2 {
		t.Fatalf("request ownership relationships %d: %+v", n, mv.Report.Lines)
	}
	repeat := l.move(t, Choices{})
	if repeat.Report.Counts[ResCreated] != 0 || l.count(t, "SELECT count(*) FROM asks WHERE ref = 'tmdb:604'") != 1 {
		t.Fatalf("repeat changed records: %+v", repeat.Report.Counts)
	}
	u := l.undo(t, mv.ID)
	if u.State != "undone" {
		t.Fatalf("undo state %s", u.State)
	}
	if n := l.count(t, "SELECT count(*) FROM settings WHERE grp = 'this_app' AND key IN ('ask_movies_per_week','ask_shows_per_week')"); n != 0 {
		t.Fatalf("moved defaults remained after undo: %d", n)
	}
	if n := l.count(t, "SELECT count(*) FROM ask_people p JOIN asks a ON a.id = p.ask_id WHERE a.ref = 'tmdb:604'"); n != 0 {
		t.Fatalf("request relationships remained after undo: %d", n)
	}
	afterBytes, err := os.ReadFile(l.file)
	if err != nil {
		t.Fatal(err)
	}
	afterInfo, err := os.Stat(l.file)
	if err != nil {
		t.Fatal(err)
	}
	if sha256.Sum256(afterBytes) != beforeHash || afterInfo.Size() != beforeInfo.Size() || !afterInfo.ModTime().Equal(beforeInfo.ModTime()) {
		t.Fatal("media fingerprint changed during preview/apply/repeat/undo")
	}
}

func TestCompatibilityAskPersonUndoRejectsReusedAskOrUserRows(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	var askID, userID, moveID int64
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		var err error
		userID, err = database.Insert(tx, "users", &domain.User{Name: "joined owner", Role: "member", State: "disabled", Perms: "[]"}, domain.System)
		if err != nil {
			return err
		}
		asked := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
		askID, err = database.Insert(tx, "asks", &domain.Ask{Kind: "movie", TMDBID: "700020", Name: "Existing", State: "asked", AskedBy: 1, AskedAt: asked, Ref: "tmdb:700020", SourceIDs: map[string]string{"tmdb": "700020"}, Seasons: []int{}, Albums: []domain.AskAlbum{}}, domain.System)
		if err != nil {
			return err
		}
		if _, err := tx.Exec("INSERT INTO ask_people (ask_id,user_id,asked_at,counted) VALUES (?,?,?,0)", askID, 1, database.FormatTime(asked)); err != nil {
			return err
		}
		res, err := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,'running')", database.FormatTime(time.Now().UTC()))
		if err == nil {
			moveID, err = res.LastInsertId()
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	r := &run{m: l.mv, ctx: ctx, move: moveID, by: domain.Actor{Kind: "user", ID: 1}, owner: 1, people: map[string]int64{"person:stable": userID}, rep: &Report{Counts: map[string]int{}}}
	it := Item{Key: "ask:seerr:nested-reuse", Name: "Existing", From: "seerr requests[0]", Ask: &AskPlan{App: "seerr", Kind: "movie", Ref: "tmdb:700020", IDs: map[string]string{"tmdb": "700020"}, By: "person:stable", AskedAt: "2026-10-02T12:00:00Z"}}
	if got, why := r.ask(it); got != ResCreated {
		t.Fatalf("ownership relation %s: %s", got, why)
	}
	var mark madeRow
	if err := l.st.Read().QueryRow("SELECT key,what,row_id,before,after,fingerprint FROM move_made WHERE move_id=?", moveID).Scan(&mark.key, &mark.what, &mark.row, &mark.before, &mark.after, &mark.fp); err != nil {
		t.Fatal(err)
	}
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE asks SET created_at=? WHERE id=?", "2099-01-01T00:00:00Z", askID); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE users SET created_at=? WHERE id=?", "2099-01-01T00:00:00Z", userID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := l.mv.undoOne(ctx, mark, domain.Actor{Kind: "user", ID: 1}); got != ResKept {
		t.Fatalf("reused nested identity was changed by undo: %s", got)
	}
	if n := l.count(t, "SELECT count(*) FROM ask_people WHERE ask_id=? AND user_id=?", askID, userID); n != 1 {
		t.Fatal("replacement request ownership relation was deleted")
	}
}

func TestCompatibilityNewAskCreationRejectsReplacedAskerAndDecider(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	var moveID, askerID, deciderID int64
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		var err error
		askerID, err = database.Insert(tx, "users", &domain.User{Name: "asker cas", Role: "member", State: "disabled", Perms: "[]"}, domain.System)
		if err != nil {
			return err
		}
		deciderID, err = database.Insert(tx, "users", &domain.User{Name: "decider cas", Role: "admin", State: "disabled", Perms: "[]"}, domain.System)
		if err != nil {
			return err
		}
		res, err := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,'running')", database.FormatTime(time.Now().UTC()))
		if err == nil {
			moveID, err = res.LastInsertId()
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	r := &run{m: l.mv, ctx: ctx, move: moveID, by: domain.Actor{Kind: "user", ID: 1}, owner: 1, people: map[string]int64{}, rep: &Report{Counts: map[string]int{}}}
	askerFP, err := userIdentity(ctx, l.st.Read(), askerID)
	if err != nil {
		t.Fatal(err)
	}
	deciderFP, err := userIdentity(ctx, l.st.Read(), deciderID)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE users SET created_at=? WHERE id=?", "2099-01-01T00:00:00.000000000Z", askerID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	asked := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
	it := Item{Key: "ask:seerr:asker-cas"}
	row := domain.Ask{Kind: "movie", TMDBID: "700030", Name: "Asker CAS", State: "asked", AskedBy: askerID, AskedAt: asked, Ref: "tmdb:700030", SourceIDs: map[string]string{"tmdb": "700030"}, Seasons: []int{}, Albums: []domain.AskAlbum{}}
	if _, err := r.createAsk(it, &row, askerID, asked, askerFP, 0, ""); !errors.Is(err, errUndoReplaced) {
		t.Fatalf("replaced asker was accepted: %v", err)
	}
	currentAskerFP, err := userIdentity(ctx, l.st.Read(), askerID)
	if err != nil {
		t.Fatal(err)
	}
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE users SET created_at=? WHERE id=?", "2099-01-02T00:00:00.000000000Z", deciderID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	decided := asked.Add(time.Hour)
	row = domain.Ask{Kind: "movie", TMDBID: "700031", Name: "Decider CAS", State: "approved", AskedBy: askerID, AskedAt: asked, DecidedBy: &deciderID, DecidedAt: &decided, Ref: "tmdb:700031", SourceIDs: map[string]string{"tmdb": "700031"}, Seasons: []int{}, Albums: []domain.AskAlbum{}}
	if _, err := r.createAsk(Item{Key: "ask:seerr:decider-cas"}, &row, askerID, asked, currentAskerFP, deciderID, deciderFP); !errors.Is(err, errUndoReplaced) {
		t.Fatalf("replaced decider was accepted: %v", err)
	}
	if n := l.count(t, "SELECT count(*) FROM asks WHERE ref IN ('tmdb:700030','tmdb:700031')"); n != 0 {
		t.Fatal("identity CAS failure left a request behind")
	}
}

func TestCompatibilityQuotasNeverWidenAndAmbiguousMusicIsReported(t *testing.T) {
	s := &Snapshot{Kind: Lidarr, Users: []User{{SourceID: 1, Name: "Slow", MovieLimit: 1, MovieDays: 30},
		{SourceID: 2, Name: "Huge", MovieLimit: 1001, MovieDays: 7, MovieLimitSet: true},
		{SourceID: 3, Name: "Negative", ShowLimit: -1, ShowDays: 7, ShowLimitSet: true}},
		Settings:         map[string]any{"default_movie_limit": int64(1001), "default_movie_days": int64(7), "default_show_limit": int64(-1), "default_show_days": int64(7)},
		MetadataProfiles: []Named{{ID: 2, Settings: map[string]any{"primaryalbumtypes": []any{map[string]any{"name": "UnknownKind", "allowed": true}}}}},
		Titles: []Title{{SourceID: 1, Kind: "artist", Name: "A", IDs: map[string]string{"musicbrainz": "056e4f3e-d505-4dad-8ec1-d04f521cbb56"},
			Extra: map[string]any{"new_albums": "mystery", "metadata_profile_id": int64(2)}, MusicAlbums: []MusicAlbum{{MBID: "bad", WatchedEdition: "also-bad"}}}}}
	p := BuildPlan([]Input{{ID: 1, Snap: s}}, Choices{})
	person := item(t, p, GroupPeople, "person:lidarr:1")
	if person.Person.MoviesPerWeek != nil {
		t.Fatal("slow quota was widened")
	}
	for _, from := range []string{"lidarr users[0].movie_quota", "lidarr users[1].movie_quota", "lidarr users[2].show_quota",
		"lidarr settings.default_movie_limit", "lidarr settings.default_show_limit", "lidarr titles[0].new_albums",
		"lidarr titles[0].metadata_profile.primaryalbumtypes", "lidarr titles[0].music_albums[0].mbid"} {
		if _, ok := notMoved(p, from); !ok {
			t.Fatalf("loss not reported: %s", from)
		}
	}
	missing := &Snapshot{Kind: Lidarr,
		MetadataProfiles: []Named{{ID: 2, Settings: map[string]any{"primaryalbumtypes": []any{}, "secondaryalbumtypes": []any{}, "releasestatuses": []any{}}}},
		Titles:           []Title{{SourceID: 2, Kind: "artist", Name: "B", IDs: map[string]string{"musicbrainz": "156e4f3e-d505-4dad-8ec1-d04f521cbb56"}, Extra: map[string]any{"metadata_profile_id": int64(2)}}}}
	mp := BuildPlan([]Input{{ID: 2, Snap: missing}}, Choices{})
	for _, from := range []string{"lidarr titles[0].new_albums", "lidarr titles[0].metadata_profile.primaryalbumtypes", "lidarr titles[0].metadata_profile.secondaryalbumtypes", "lidarr titles[0].metadata_profile.releasestatuses"} {
		if _, ok := notMoved(mp, from); !ok {
			t.Fatalf("missing policy field not reported: %s", from)
		}
	}
	noProfile := &Snapshot{Kind: Lidarr, Folders: []Folder{{Path: "/music"}}, Titles: []Title{{SourceID: 3, Kind: "artist", Name: "C", Path: "/music/C", IDs: map[string]string{"musicbrainz": "256e4f3e-d505-4dad-8ec1-d04f521cbb56"}, Extra: map[string]any{"new_albums": "new"}}}}
	np := BuildPlan([]Input{{ID: 3, Snap: noProfile}}, Choices{PathRules: []PathRule{{From: "/music", To: "C:/music"}}})
	if _, ok := notMoved(np, "lidarr titles[0].metadata_profile_id"); !ok {
		t.Fatal("missing metadata profile was not reported")
	}
}

func TestCompatibilitySourceTimeParsingKeepsDecisionIdentity(t *testing.T) {
	if got, ok := sourceTime("2026-10-03"); !ok || got.Format("2006-01-02") != "2026-10-03" {
		t.Fatal("backup date not parsed")
	}
	rq, _ := seerrRequest(1, 2, "movie", map[string]any{"tmdbId": 603}, 2, 2, false, "2026-10-01", "2026-10-03")
	if rq.DecidedBy != 2 {
		t.Fatal("self approval identity lost")
	}
}

func TestCompatibilityLegacyPersonMarkCannotRecycleIdentity(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	var uid int64
	err := l.st.Write(ctx, func(tx *sql.Tx) error {
		var e error
		uid, e = database.Insert(tx, "users", &domain.User{Name: "legacy", Role: "member", State: "disabled", Perms: "[]"}, domain.System)
		if e != nil {
			return e
		}
		res, e := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,'done')", database.FormatTime(time.Now().UTC()))
		if e != nil {
			return e
		}
		mid, _ := res.LastInsertId()
		_, e = tx.Exec("INSERT INTO move_made (move_id,key,what,row_id,created_at) VALUES (?,'person:seerr:2','user',?,?)", mid, uid, database.FormatTime(time.Now().UTC()))
		return e
	})
	if err != nil {
		t.Fatal(err)
	}
	var replacement int64
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("DELETE FROM user_ask_settings WHERE user_id=?", uid); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM users WHERE id=?", uid); err != nil {
			return err
		}
		var err error
		replacement, err = database.Insert(tx, "users", &domain.User{Name: "replacement", Role: "member", State: "disabled", Perms: "[]"}, domain.System)
		if err != nil {
			return err
		}
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if replacement != uid {
		t.Fatalf("fixture did not reuse id: old=%d replacement=%d", uid, replacement)
	}
	r := &run{m: l.mv, ctx: ctx, move: 1, by: domain.Actor{Kind: "user", ID: 1}, owner: 1, people: map[string]int64{}}
	it := Item{Key: "person:seerr:plex:stable", LegacyKeys: []string{"person:seerr:2"}, Name: "Imported", Person: &PersonPlan{App: "seerr", SourceID: 2, StableIdentity: true, Role: "member", Perms: []string{}}}
	res, why := r.one(GroupPeople, it)
	if res != ResSkipped || r.people[it.Key] != -2 || r.choose != ChooseNumericID {
		t.Fatalf("legacy mark did not require an explicit identity choice: %s %q %v", res, why, r.people)
	}
	ask := Item{Key: "ask:seerr:reused-source-id", Name: "Wrong owner guard", Ask: &AskPlan{App: "seerr", Kind: "movie", Ref: "tmdb:700010", IDs: map[string]string{"tmdb": "700010"}, By: it.Key}}
	if got, _ := r.ask(ask); got != ResSkipped {
		t.Fatalf("request with unverifiable owner was not skipped: %s", got)
	}
	if n := l.count(t, "SELECT count(*) FROM asks WHERE ref='tmdb:700010'"); n != 0 {
		t.Fatal("request was attached despite an unverifiable source identity")
	}
}

func TestCompatibilityLegacyPersonMarkMissingRowRetries(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		now := database.FormatTime(time.Now().UTC())
		res, err := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,'done')", now)
		if err != nil {
			return err
		}
		moveID, _ := res.LastInsertId()
		_, err = tx.Exec("INSERT INTO move_made (move_id,key,what,row_id,created_at) VALUES (?,'person:seerr:3','user',999999,?)", moveID, now)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	r := &run{m: l.mv, ctx: ctx, move: 1, by: domain.Actor{Kind: "user", ID: 1}, owner: 1, people: map[string]int64{}}
	it := Item{Key: "person:seerr:stable-3", LegacyKeys: []string{"person:seerr:3"}, Name: "Imported missing", Person: &PersonPlan{App: "seerr", SourceID: 3, StableIdentity: true, Role: "member", Perms: []string{}}}
	if got, why := r.one(GroupPeople, it); got != ResSkipped || r.choose != ChooseNumericID {
		t.Fatalf("missing legacy identity was not failed closed: %s %s", got, why)
	}

	// A test run asks for the choice in its next steps.
	plan := func(choice string) *Plan {
		p := *it.Person
		p.Choice = choice
		item := it
		item.Person = &p
		return &Plan{Apps: []string{"seerr"}, Groups: []Group{{Name: GroupPeople, Label: GroupLabels[GroupPeople], Items: []Item{item}}}}
	}
	rep := l.mv.Run(ctx, 0, plan(""), true, domain.Actor{Kind: "user", ID: 1}, nil)
	if got := lineOf(t, rep, it.Key); got.Result != ResSkipped || got.Choose != ChooseNumericID {
		t.Fatalf("numeric-id line: %+v", got)
	}
	if !hasNext(rep, "Some people need a choice") {
		t.Fatalf("next steps do not ask for the choice: %v", rep.Next)
	}
	// Same person with no one of that name here stays a numeric-id choice.
	rep = l.mv.Run(ctx, 0, plan(PersonMerge), true, domain.Actor{Kind: "user", ID: 1}, nil)
	if got := lineOf(t, rep, it.Key); got.Result != ResSkipped || got.Choose != ChooseNumericID || strings.Contains(got.Detail, "no longer here") {
		t.Fatalf("Same person on a numeric-id person: %+v", got)
	}
	if !hasNext(rep, "Some people need a choice") {
		t.Fatalf("next steps do not ask for the choice after Same person: %v", rep.Next)
	}
	// Leave out closes the choice.
	rep = l.mv.Run(ctx, 0, plan(PersonSkip), true, domain.Actor{Kind: "user", ID: 1}, nil)
	if hasNext(rep, "Some people need a choice") {
		t.Fatalf("next steps still ask after Leave out: %v", rep.Next)
	}
}

func TestCompatibilityRenamedStablePersonIsReused(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	newMove := func() int64 {
		var id int64
		if err := l.st.Write(ctx, func(tx *sql.Tx) error {
			res, err := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,'running')", database.FormatTime(time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)))
			if err == nil {
				id, err = res.LastInsertId()
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	it := Item{Key: "person:seerr:plex:stable-7", Name: "Before", From: "seerr users[0]", Person: &PersonPlan{App: "seerr", SourceID: 7, StableIdentity: true, Role: "member", Perms: []string{}}}
	r1 := &run{m: l.mv, ctx: ctx, move: newMove(), by: domain.Actor{Kind: "user", ID: 1}, owner: 1, people: map[string]int64{}}
	if got, why := r1.person(it); got != ResCreated {
		t.Fatalf("person create %s: %s", got, why)
	}
	personID := r1.people[it.Key]
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE users SET name='After' WHERE id=?", personID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	r2 := &run{m: l.mv, ctx: ctx, move: newMove(), by: domain.Actor{Kind: "user", ID: 1}, owner: 1, people: map[string]int64{}}
	if got, _ := r2.one(GroupPeople, it); got != ResAlready || r2.people[it.Key] != personID {
		t.Fatalf("renamed stable person not reused: %s %+v", got, r2.people)
	}
	if n := l.count(t, "SELECT count(*) FROM users WHERE role='member'"); n != 1 {
		t.Fatalf("renamed repeat created %d members", n)
	}
	ask := Item{Key: "ask:seerr:stable-person", Name: "Owned", From: "seerr requests[0]", Ask: &AskPlan{App: "seerr", Kind: "movie", Ref: "tmdb:700003", IDs: map[string]string{"tmdb": "700003"}, By: it.Key}}
	if got, why := r2.ask(ask); got != ResCreated {
		t.Fatalf("ask create %s: %s", got, why)
	}
	if n := l.count(t, "SELECT count(*) FROM asks a JOIN ask_people p ON p.ask_id=a.id WHERE a.ref='tmdb:700003' AND a.asked_by=? AND p.user_id=?", personID, personID); n != 1 {
		t.Fatal("request ownership drifted after person rename")
	}
}

func TestCompatibilityReusedUserIDDoesNotStealStableIdentity(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	newMove := func() int64 {
		var id int64
		if err := l.st.Write(ctx, func(tx *sql.Tx) error {
			res, err := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,'running')", database.FormatTime(time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)))
			if err == nil {
				id, err = res.LastInsertId()
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	it := Item{Key: "person:seerr:plex:reuse-8", Name: "Imported", From: "seerr users[0]", Person: &PersonPlan{App: "seerr", SourceID: 8, StableIdentity: true, Role: "member", Perms: []string{}}}
	r1 := &run{m: l.mv, ctx: ctx, move: newMove(), by: domain.Actor{Kind: "user", ID: 1}, owner: 1, people: map[string]int64{}}
	if got, why := r1.person(it); got != ResCreated {
		t.Fatalf("person create %s: %s", got, why)
	}
	oldID := r1.people[it.Key]
	var replacementID int64
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("DELETE FROM user_ask_settings WHERE user_id=?", oldID); err != nil {
			return err
		}
		if _, err := tx.Exec("DELETE FROM users WHERE id=?", oldID); err != nil {
			return err
		}
		var err error
		replacementID, err = database.Insert(tx, "users", &domain.User{Name: "Replacement", Role: "member", State: "disabled", Perms: "[]"}, domain.System)
		if err != nil {
			return err
		}
		_, err = tx.Exec("UPDATE users SET created_at=? WHERE id=?", database.FormatTime(time.Date(2026, 10, 15, 13, 0, 0, 0, time.UTC)), replacementID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if replacementID != oldID {
		t.Fatalf("fixture did not reuse id: old=%d replacement=%d", oldID, replacementID)
	}
	r2 := &run{m: l.mv, ctx: ctx, move: newMove(), by: domain.Actor{Kind: "user", ID: 1}, owner: 1, people: map[string]int64{}}
	if got, why := r2.one(GroupPeople, it); got != ResCreated {
		t.Fatalf("replacement id blocked import %s: %s", got, why)
	}
	if r2.people[it.Key] == replacementID {
		t.Fatal("stable identity mapped to unrelated replacement user")
	}
}

func TestCompatibilityMusicUndoRejectsReplacedTitle(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	var titleID, moveID int64
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		var err error
		titleID, err = database.Insert(tx, "titles", &mediafiles.Title{Kind: "artist", Name: "Original", SortName: "original", Aliases: []string{}, SourceIDs: map[string]string{"musicbrainz": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}, MetadataSource: "musicbrainz", Artwork: []string{}, Status: "running", State: "active", ReleaseGate: "released", GateDriver: "none", NewPartsPolicy: "all", WhyAdded: "migration", MusicTypes: []string{}, MusicSecondary: []string{}, MusicStatuses: []string{}}, domain.System)
		if err != nil {
			return err
		}
		res, err := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,'running')", database.FormatTime(time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)))
		if err == nil {
			moveID, err = res.LastInsertId()
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	state := musicState{NewAlbums: "all", Types: "[]", Secondary: "[]", Statuses: "[]", Parts: []musicPartState{}}
	j, _ := json.Marshal(state)
	r := &run{m: l.mv, ctx: ctx, move: moveID}
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		return r.markTx(tx, "title:artist:replacement", "music_policy", titleID, string(j), string(j))
	}); err != nil {
		t.Fatal(err)
	}
	var mark madeRow
	if err := l.st.Read().QueryRow("SELECT key,what,row_id,before,after,fingerprint FROM move_made WHERE move_id=?", moveID).Scan(&mark.key, &mark.what, &mark.row, &mark.before, &mark.after, &mark.fp); err != nil {
		t.Fatal(err)
	}
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE titles SET name='Replacement', created_at=? WHERE id=?", database.FormatTime(time.Date(2026, 10, 15, 13, 0, 0, 0, time.UTC)), titleID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := l.mv.undoOne(ctx, mark, domain.Actor{Kind: "user", ID: 1}); got != ResKept {
		t.Fatalf("replacement title was mutated: %s", got)
	}
	if n := l.count(t, "SELECT count(*) FROM titles WHERE id=? AND name='Replacement'", titleID); n != 1 {
		t.Fatal("replacement title changed during undo")
	}
}

func TestCompatibilityDecisionTimeGapsAndStaleTitleMarksAreExplicit(t *testing.T) {
	readAt := time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)
	s := &Snapshot{Kind: Seerr, ReadAt: readAt,
		Users: []User{{SourceID: 1, Name: "Owner", Owner: true}},
		Requests: []Request{
			{SourceID: 1, Kind: "movie", IDs: map[string]string{"tmdb": "603"}, State: "approved", By: 1},
			{SourceID: 2, Kind: "movie", IDs: map[string]string{"tmdb": "604"}, State: "approved", By: 1, Updated: "2999-01-01T00:00:00Z"},
			{SourceID: 3, Kind: "movie", IDs: map[string]string{"tmdb": "605"}, State: "approved", By: 1, Updated: "2026-10-15T11:59:59Z"},
		}}
	p := BuildPlan([]Input{{ID: 1, Snap: s}}, Choices{})
	if _, ok := notMoved(p, "seerr requests[0].created"); !ok {
		t.Fatal("missing request time loss not reported")
	}
	if _, ok := notMoved(p, "seerr requests[0].updated"); !ok {
		t.Fatal("missing decision time loss not reported")
	}
	if _, ok := notMoved(p, "seerr requests[1].updated"); !ok {
		t.Fatal("future decision time was not reported")
	}
	if got := item(t, p, GroupRequests, "ask:seerr:2").Ask.DecidedAt; got != "" {
		t.Fatalf("future decision time survived in plan: %q", got)
	}
	if got := item(t, p, GroupRequests, "ask:seerr:3").Ask.DecidedAt; got != "2026-10-15T11:59:59Z" {
		t.Fatalf("captured decision time lost: %q", got)
	}
	if h1, h2 := PlanHash(BuildPlan([]Input{{ID: 1, Snap: s}}, Choices{})), PlanHash(BuildPlan([]Input{{ID: 1, Snap: s}}, Choices{})); h1 != h2 {
		t.Fatalf("wall clock changed plan hash: %s != %s", h1, h2)
	}
	legacy := &Snapshot{Kind: Seerr, Requests: []Request{{SourceID: 4, Kind: "movie", IDs: map[string]string{"tmdb": "606"}, Created: "2026-10-01T00:00:00Z"}}}
	lp := BuildPlan([]Input{{ID: 2, Snap: legacy}}, Choices{})
	if _, ok := notMoved(lp, "seerr requests[0].created"); !ok || item(t, lp, GroupRequests, "ask:seerr:4").Ask.AskedAt != "" {
		t.Fatal("timestamp without a persisted capture bound was not left explicit")
	}

	l := newMoveLab(t)
	ctx := context.Background()
	var titleID, moveID int64
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		var err error
		titleID, err = database.Insert(tx, "titles", &mediafiles.Title{Kind: "movie", Name: "Removed", SortName: "removed", Aliases: []string{}, SourceIDs: map[string]string{"tmdb": "999999"}, MetadataSource: "tmdb", Artwork: []string{}, State: "removed", Status: "released", ReleaseGate: "released", GateDriver: "none", NewPartsPolicy: "all", WhyAdded: "migration"}, domain.System)
		if err != nil {
			return err
		}
		res, err := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,'done')", database.FormatTime(time.Now().UTC()))
		if err != nil {
			return err
		}
		moveID, _ = res.LastInsertId()
		fp := fingerprint(ctx, tx, "title", titleID)
		_, err = tx.Exec("INSERT INTO move_made (move_id,key,what,row_id,fingerprint,created_at) VALUES (?,'title:movie:tmdb:999999','title',?,?,?)", moveID, titleID, fp, database.FormatTime(time.Now().UTC()))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	r := &run{m: l.mv, ctx: ctx}
	if _, ok := r.made("title:movie:tmdb:999999"); ok {
		t.Fatal("removed title's stale mark blocked a retry")
	}
}

func TestCompatibilityFractionalSourceTimeBeforeCaptureIsRetained(t *testing.T) {
	s := &Snapshot{Kind: Seerr, ReadAt: time.Date(2026, 10, 15, 12, 0, 0, 900_000_000, time.UTC),
		Requests: []Request{{SourceID: 1, Kind: "movie", IDs: map[string]string{"tmdb": "700004"}, Created: "2026-10-15T12:00:00.500Z"}}}
	p := BuildPlan([]Input{{ID: 1, Snap: s}}, Choices{})
	if got := item(t, p, GroupRequests, "ask:seerr:1").Ask.AskedAt; got != "2026-10-15T12:00:00.500Z" {
		t.Fatalf("valid same-second source time was rejected: %q", got)
	}
}

func TestCompatibilityAskUndoPreservesLaterPeople(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	move := func() int64 {
		var id int64
		if err := l.st.Write(ctx, func(tx *sql.Tx) error {
			res, err := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,'running')", database.FormatTime(time.Date(2026, 10, 15, 12, 0, 0, 0, time.UTC)))
			if err == nil {
				id, err = res.LastInsertId()
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	mark := func(moveID int64) madeRow {
		x := madeRow{move: moveID}
		if err := l.st.Read().QueryRow("SELECT key, what, row_id, before, after, fingerprint FROM move_made WHERE move_id=?", moveID).Scan(&x.key, &x.what, &x.row, &x.before, &x.after, &x.fp); err != nil {
			t.Fatal(err)
		}
		return x
	}
	makeAsk := func(moveID int64, key, ref string) madeRow {
		r := &run{m: l.mv, ctx: ctx, move: moveID, by: domain.Actor{Kind: "user", ID: 1}, owner: 1, people: map[string]int64{}}
		it := Item{Key: key, Name: ref, From: "seerr requests[0]", Ask: &AskPlan{App: "seerr", Kind: "movie", Ref: ref, IDs: map[string]string{"tmdb": ref[5:]}}}
		if got, why := r.ask(it); got != ResCreated {
			t.Fatalf("create ask %s: %s", got, why)
		}
		return mark(moveID)
	}

	moveA := move()
	markA := makeAsk(moveA, "ask:seerr:a", "tmdb:700001")
	var personB int64
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		var err error
		personB, err = database.Insert(tx, "users", &domain.User{Name: "later", Role: "member", State: "disabled", Perms: "[]"}, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	moveB := move()
	rB := &run{m: l.mv, ctx: ctx, move: moveB, by: domain.Actor{Kind: "user", ID: 1}, owner: 1, people: map[string]int64{"person:b": personB}}
	if got, why := rB.ask(Item{Key: "ask:seerr:b", Name: "shared", From: "seerr requests[1]", Ask: &AskPlan{App: "seerr", Kind: "movie", Ref: "tmdb:700001", IDs: map[string]string{"tmdb": "700001"}, By: "person:b"}}); got != ResCreated {
		t.Fatalf("join ask %s: %s", got, why)
	}
	markB := mark(moveB)
	if got, _ := l.mv.undoOne(ctx, markA, domain.Actor{Kind: "user", ID: 1}); got != ResKept {
		t.Fatalf("earlier undo removed later relationship: %s", got)
	}
	moveB2 := move()
	rB2 := &run{m: l.mv, ctx: ctx, move: moveB2, by: domain.Actor{Kind: "user", ID: 1}, owner: 1, people: map[string]int64{"person:b": personB}}
	if got, why := rB2.ask(Item{Key: "ask:seerr:b-again", Name: "shared", From: "seerr requests[2]", Ask: &AskPlan{App: "seerr", Kind: "movie", Ref: "tmdb:700001", IDs: map[string]string{"tmdb": "700001"}, By: "person:b"}}); got != ResAlready {
		t.Fatalf("same-owner relation claim %s: %s", got, why)
	}
	markB2 := mark(moveB2)
	if got, _ := l.mv.undoOne(ctx, markB, domain.Actor{Kind: "user", ID: 1}); got != ResKept {
		t.Fatalf("earlier ownership link erased a later same-owner claim: %s", got)
	}
	if got, _ := l.mv.undoOne(ctx, markB2, domain.Actor{Kind: "user", ID: 1}); got != ResRemoved {
		t.Fatalf("same-owner relation claim undo %s", got)
	}
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE moves SET state='undone' WHERE id=?", moveB2)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE ask_people SET asked_at=? WHERE ask_id=? AND user_id=?", database.FormatTime(time.Date(2026, 10, 15, 13, 0, 0, 0, time.UTC)), markA.row, personB)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := l.mv.undoOne(ctx, markB, domain.Actor{Kind: "user", ID: 1}); got != ResKept {
		t.Fatalf("changed later relationship was removed: %s", got)
	}
	var markBIdentity askPersonMark
	if err := json.Unmarshal([]byte(markB.before), &markBIdentity); err != nil {
		t.Fatal(err)
	}
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE ask_people SET asked_at=? WHERE ask_id=? AND user_id=?", markBIdentity.AskedAt, markA.row, personB)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := l.mv.undoOne(ctx, markB, domain.Actor{Kind: "user", ID: 1}); got != ResRemoved {
		t.Fatalf("later relationship undo %s", got)
	}
	if got, _ := l.mv.undoOne(ctx, markA, domain.Actor{Kind: "user", ID: 1}); got != ResRemoved {
		t.Fatalf("original ask undo after later undo %s", got)
	}

	moveC := move()
	markC := makeAsk(moveC, "ask:seerr:c", "tmdb:700002")
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("INSERT INTO ask_people (ask_id,user_id,asked_at,counted) VALUES (?,?,?,0)", markC.row, personB, database.FormatTime(time.Date(2026, 10, 15, 13, 0, 0, 0, time.UTC)))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := l.mv.undoOne(ctx, markC, domain.Actor{Kind: "user", ID: 1}); got != ResKept {
		t.Fatalf("real later join was removed: %s", got)
	}
	if n := l.count(t, "SELECT count(*) FROM ask_people WHERE ask_id=?", markC.row); n != 2 {
		t.Fatalf("real join relationship count %d", n)
	}

	moveD := move()
	markD := makeAsk(moveD, "ask:seerr:same-owner", "tmdb:700005")
	moveE := move()
	rE := &run{m: l.mv, ctx: ctx, move: moveE, by: domain.Actor{Kind: "user", ID: 1}, owner: 1, people: map[string]int64{}}
	same := Item{Key: "ask:seerr:same-owner", Name: "same owner", From: "seerr requests[2]", Ask: &AskPlan{App: "seerr", Kind: "movie", Ref: "tmdb:700005", IDs: map[string]string{"tmdb": "700005"}}}
	if got, why := rE.one(GroupRequests, same); got != ResAlready {
		t.Fatalf("later same-owner move %s: %s", got, why)
	}
	claim := mark(moveE)
	if claim.what != "ask_claim" || claim.row != markD.row {
		t.Fatalf("later move did not retain a dependency claim: %+v", claim)
	}
	if got, _ := l.mv.undoOne(ctx, markD, domain.Actor{Kind: "user", ID: 1}); got != ResKept {
		t.Fatalf("earlier undo erased later same-owner import: %s", got)
	}
	if got, _ := l.mv.undoOne(ctx, claim, domain.Actor{Kind: "user", ID: 1}); got != ResRemoved {
		t.Fatalf("claim undo failed: %s", got)
	}
	if n := l.count(t, "SELECT count(*) FROM ask_people WHERE ask_id=? AND user_id=1", markD.row); n != 1 {
		t.Fatal("claim undo deleted the pre-existing request ownership")
	}

	moveF := move()
	rF := &run{m: l.mv, ctx: ctx, move: moveF, by: domain.Actor{Kind: "user", ID: 1}, owner: 1, people: map[string]int64{}}
	repeated := Item{Key: "ask:seerr:same-move", Name: "same move", From: "seerr requests[3]", Ask: &AskPlan{App: "seerr", Kind: "movie", Ref: "tmdb:700006", IDs: map[string]string{"tmdb": "700006"}}}
	if got, why := rF.one(GroupRequests, repeated); got != ResCreated {
		t.Fatalf("same-move first apply %s: %s", got, why)
	}
	firstMark := mark(moveF)
	if got, why := rF.one(GroupRequests, repeated); got != ResAlready {
		t.Fatalf("same-move repeat %s: %s", got, why)
	}
	secondMark := mark(moveF)
	if secondMark.what != "ask" || secondMark.row != firstMark.row || secondMark.after != firstMark.after {
		t.Fatalf("same-move repeat replaced its undo mark: before=%+v after=%+v", firstMark, secondMark)
	}
	var repeatedBaseline askBaseline
	if err := json.Unmarshal([]byte(firstMark.after), &repeatedBaseline); err != nil {
		t.Fatal(err)
	}
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE ask_people SET asked_at=? WHERE ask_id=? AND user_id=?", "2099-01-01T00:00:00Z", firstMark.row, repeatedBaseline.UserID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := rF.one(GroupRequests, repeated); got != ResFailed {
		t.Fatalf("same-move repeat trusted a changed relationship: %s", got)
	}
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE ask_people SET asked_at=? WHERE ask_id=? AND user_id=?", repeatedBaseline.AskedAt, firstMark.row, repeatedBaseline.UserID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	originalAskCreated := strings.SplitN(firstMark.fp, string(rune(31)), 2)[0]
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE asks SET created_at=? WHERE id=?", "2099-01-01T00:00:00.000000000Z", firstMark.row)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := rF.one(GroupRequests, repeated); got != ResFailed {
		t.Fatalf("same-move repeat trusted a replaced ask: %s", got)
	}
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		if _, err := tx.Exec("UPDATE asks SET created_at=? WHERE id=?", originalAskCreated, firstMark.row); err != nil {
			return err
		}
		_, err := tx.Exec("UPDATE users SET created_at=? WHERE id=?", "2099-01-01T00:00:00.000000000Z", repeatedBaseline.UserID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := rF.one(GroupRequests, repeated); got != ResFailed {
		t.Fatalf("same-move repeat trusted a replaced request owner: %s", got)
	}
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE users SET created_at=? WHERE id=?", repeatedBaseline.UserFingerprint, repeatedBaseline.UserID)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := l.mv.undoOne(ctx, secondMark, domain.Actor{Kind: "user", ID: 1}); got != ResRemoved {
		t.Fatalf("same-move repeated ask undo %s", got)
	}
}

func TestCompatibilitySameMoveAskClaimsDoNotBlockFullUndo(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	newMove := func() int64 {
		var id int64
		if err := l.st.Write(ctx, func(tx *sql.Tx) error {
			res, err := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,'running')", database.FormatTime(time.Now().UTC()))
			if err == nil {
				id, err = res.LastInsertId()
			}
			return err
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	finish := func(id int64) {
		if err := l.st.Write(ctx, func(tx *sql.Tx) error {
			_, err := tx.Exec("UPDATE moves SET state='done' WHERE id=?", id)
			return err
		}); err != nil {
			t.Fatal(err)
		}
	}
	actor := domain.Actor{Kind: "user", ID: 1}
	moveA := newMove()
	rA := &run{m: l.mv, ctx: ctx, move: moveA, by: actor, owner: 1, people: map[string]int64{}, rep: &Report{Counts: map[string]int{}}}
	first := Item{Key: "ask:seerr:same-move-a", Name: "same move ask", Ask: &AskPlan{App: "seerr", Kind: "movie", Ref: "tmdb:700040", IDs: map[string]string{"tmdb": "700040"}}}
	second := first
	second.Key = "ask:seerr:same-move-b"
	if got, why := rA.one(GroupRequests, first); got != ResCreated {
		t.Fatalf("same-move ask create %s: %s", got, why)
	}
	if got, why := rA.one(GroupRequests, second); got != ResAlready {
		t.Fatalf("same-move ask claim %s: %s", got, why)
	}
	finish(moveA)
	if _, err := l.mv.Undo(ctx, moveA, actor); err != nil {
		t.Fatal(err)
	}
	if n := l.count(t, "SELECT count(*) FROM asks WHERE ref='tmdb:700040'"); n != 0 {
		t.Fatal("same-move ask claim blocked removal of the move-created ask")
	}

	var existingAsk, personID int64
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		var err error
		personID, err = database.Insert(tx, "users", &domain.User{Name: "same move relation", Role: "member", State: "disabled", Perms: "[]"}, domain.System)
		if err != nil {
			return err
		}
		asked := time.Date(2026, 10, 1, 12, 0, 0, 0, time.UTC)
		existingAsk, err = database.Insert(tx, "asks", &domain.Ask{Kind: "movie", TMDBID: "700041", Name: "existing", State: "asked", AskedBy: 1, AskedAt: asked, Ref: "tmdb:700041", SourceIDs: map[string]string{"tmdb": "700041"}, Seasons: []int{}, Albums: []domain.AskAlbum{}}, domain.System)
		if err != nil {
			return err
		}
		_, err = tx.Exec("INSERT INTO ask_people (ask_id,user_id,asked_at,counted) VALUES (?,?,?,0)", existingAsk, 1, database.FormatTime(asked))
		return err
	}); err != nil {
		t.Fatal(err)
	}
	moveB := newMove()
	rB := &run{m: l.mv, ctx: ctx, move: moveB, by: actor, owner: 1, people: map[string]int64{"person:b": personID}, rep: &Report{Counts: map[string]int{}}}
	join1 := Item{Key: "ask:seerr:same-relation-a", Name: "existing", Ask: &AskPlan{App: "seerr", Kind: "movie", Ref: "tmdb:700041", IDs: map[string]string{"tmdb": "700041"}, By: "person:b"}}
	join2 := join1
	join2.Key = "ask:seerr:same-relation-b"
	if got, why := rB.one(GroupRequests, join1); got != ResCreated {
		t.Fatalf("same-move relation create %s: %s", got, why)
	}
	if got, why := rB.one(GroupRequests, join2); got != ResAlready {
		t.Fatalf("same-move relation claim %s: %s", got, why)
	}
	finish(moveB)
	if _, err := l.mv.Undo(ctx, moveB, actor); err != nil {
		t.Fatal(err)
	}
	if n := l.count(t, "SELECT count(*) FROM asks WHERE id=?", existingAsk); n != 1 {
		t.Fatal("undo removed an ask that existed before the move")
	}
	if n := l.count(t, "SELECT count(*) FROM ask_people WHERE ask_id=? AND user_id=?", existingAsk, personID); n != 0 {
		t.Fatal("same-move claim blocked removal of the move-created ownership relation")
	}
	if n := l.count(t, "SELECT count(*) FROM ask_people WHERE ask_id=? AND user_id=1", existingAsk); n != 1 {
		t.Fatal("undo changed the pre-existing ownership relation")
	}
}

func TestCompatibilitySettingReadErrorsAreNotMissing(t *testing.T) {
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(filepath.Join(t.TempDir(), "closed.db")))
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	if got, err := readSettingValue(context.Background(), db, "ask_movies_per_week"); err == nil || got.Exists {
		t.Fatalf("database error became a missing value: %+v %v", got, err)
	}
}

func TestCompatibilityMissingRequestTimesUsePersistedMoveTime(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	moveAt := time.Date(2026, 10, 15, 14, 30, 0, 0, time.UTC)
	var moveID int64
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		res, err := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,'running')", database.FormatTime(moveAt))
		if err == nil {
			moveID, err = res.LastInsertId()
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	r := &run{m: l.mv, ctx: ctx, by: domain.Actor{Kind: "user", ID: 1}, move: moveID, people: map[string]int64{}, owner: 1}
	it := Item{Key: "ask:seerr:missing-time", Name: "Example", From: "seerr requests[0]", Ask: &AskPlan{App: "seerr", Kind: "movie", Ref: "tmdb:604", IDs: map[string]string{"tmdb": "604"}}}
	if got, why := r.ask(it); got != ResCreated {
		t.Fatalf("first apply %s: %s", got, why)
	}
	var asked string
	if err := l.st.Read().QueryRow("SELECT asked_at FROM ask_people WHERE ask_id = (SELECT id FROM asks WHERE ref='tmdb:604')").Scan(&asked); err != nil {
		t.Fatal(err)
	}
	if asked != database.FormatTime(moveAt) {
		t.Fatalf("missing source time used %q, want persisted move time %q", asked, database.FormatTime(moveAt))
	}
	r.moveAt = time.Time{}
	l.mv.Now = func() time.Time { return moveAt.Add(48 * time.Hour) }
	if got, _ := r.ask(it); got != ResAlready {
		t.Fatalf("retry was not stable: %s", got)
	}
}

func TestCompatibilityAskSettingsUndoRestoresRawValue(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	var moveID int64
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		now := database.FormatTime(time.Now().UTC())
		if _, err := tx.Exec("INSERT INTO settings (grp,key,value,created_at,created_by,updated_at,updated_by) VALUES ('this_app','ask_movies_per_week','custom-raw',?,'{}',?,'{}')", now, now); err != nil {
			return err
		}
		res, err := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,'running')", now)
		if err == nil {
			moveID, err = res.LastInsertId()
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	four := 4
	r := &run{m: l.mv, ctx: ctx, by: domain.Actor{Kind: "user", ID: 1}, move: moveID}
	if got, why := r.askSettings(Item{Key: "wishlist_defaults", AskSettings: &AskSettingsPlan{MoviesPerWeek: &four}}); got != ResChanged {
		t.Fatalf("settings apply %s: %s", got, why)
	}
	var mark madeRow
	if err := l.st.Read().QueryRow("SELECT key, what, row_id, before, after, fingerprint FROM move_made WHERE move_id=?", moveID).Scan(&mark.key, &mark.what, &mark.row, &mark.before, &mark.after, &mark.fp); err != nil {
		t.Fatal(err)
	}
	if got, why := l.mv.undoOne(ctx, mark, domain.Actor{Kind: "user", ID: 1}); got != ResRemoved {
		t.Fatalf("settings undo %s: %s", got, why)
	}
	var raw string
	if err := l.st.Read().QueryRow("SELECT value FROM settings WHERE grp='this_app' AND key='ask_movies_per_week'").Scan(&raw); err != nil || raw != "custom-raw" {
		t.Fatalf("raw setting not restored: %q %v", raw, err)
	}
}

func TestCompatibilityAskSettingsUndoKeepsTransactionLocalChange(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	var moveID int64
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		now := database.FormatTime(time.Now().UTC())
		res, err := tx.Exec("INSERT INTO moves (created_at, owner_id, state) VALUES (?,1,'running')", now)
		if err == nil {
			moveID, err = res.LastInsertId()
		}
		return err
	}); err != nil {
		t.Fatal(err)
	}
	four := 4
	r := &run{m: l.mv, ctx: ctx, by: domain.Actor{Kind: "user", ID: 1}, move: moveID}
	if got, why := r.askSettings(Item{Key: "wishlist_defaults", AskSettings: &AskSettingsPlan{MoviesPerWeek: &four}}); got != ResChanged {
		t.Fatalf("settings apply %s: %s", got, why)
	}
	var mark madeRow
	if err := l.st.Read().QueryRow("SELECT key, what, row_id, before, after, fingerprint FROM move_made WHERE move_id=?", moveID).Scan(&mark.key, &mark.what, &mark.row, &mark.before, &mark.after, &mark.fp); err != nil {
		t.Fatal(err)
	}
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		_, err := tx.Exec("UPDATE settings SET value='5' WHERE grp='this_app' AND key='ask_movies_per_week'")
		return err
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := l.mv.undoOne(ctx, mark, domain.Actor{Kind: "user", ID: 1}); got != ResKept {
		t.Fatalf("changed setting was overwritten by undo: %s", got)
	}
	var value string
	if err := l.st.Read().QueryRow("SELECT value FROM settings WHERE grp='this_app' AND key='ask_movies_per_week'").Scan(&value); err != nil || value != "5" {
		t.Fatalf("changed setting did not stay: %q %v", value, err)
	}
}

func TestCompatibilityMusicRestoreRollsBackWhenExpectedRowDisappears(t *testing.T) {
	l := newMoveLab(t)
	ctx := context.Background()
	var titleID int64
	if err := l.st.Write(ctx, func(tx *sql.Tx) error {
		var err error
		titleID, err = database.Insert(tx, "titles", &mediafiles.Title{Kind: "artist", Name: "Artist", SortName: "artist", Aliases: []string{}, SourceIDs: map[string]string{"musicbrainz": "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"}, MetadataSource: "musicbrainz", Artwork: []string{}, Status: "running", State: "active", ReleaseGate: "released", GateDriver: "none", NewPartsPolicy: "all", WhyAdded: "migration", MusicTypes: []string{}, MusicSecondary: []string{}, MusicStatuses: []string{}}, domain.System)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	state := musicState{KeepUpToDate: false, NewAlbums: "none", Types: "[]", Secondary: "[]", Statuses: "[]", Parts: []musicPartState{{PartID: 999999}}}
	err := l.st.Write(ctx, func(tx *sql.Tx) error {
		return restoreMusicState(tx, titleID, state, domain.Actor{Kind: "user", ID: 1}, database.FormatTime(time.Now().UTC()))
	})
	if !errors.Is(err, errUndoChanged) {
		t.Fatalf("missing expected music row did not abort restore: %v", err)
	}
	if n := l.count(t, "SELECT count(*) FROM titles WHERE id=? AND new_parts_policy='all'", titleID); n != 1 {
		t.Fatal("partial music restore committed before the missing-row failure")
	}
}
