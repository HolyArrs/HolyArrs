package main

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// route: GET /live
// The live 'ask' copy an approver receives carries who else declined
// the same title, as the list read does, so a live update never drops it.
// The asker's own live copy carries no also_declined and no sign-in name of
// the decider.
func TestAlsoDeclinedLiveForApproversOnly(t *testing.T) {
	t.Parallel()
	r := newAskRig(t)
	first := askOf(t, r.ana.call("POST", "/api/v1/asks", `{"id":"tmdb:693134"}`, 201))
	r.owner.call("POST", "/api/v1/asks/"+itoa(first.ID)+"/turn-down", `{}`, 200)
	again := askOf(t, r.bo.call("POST", "/api/v1/asks", `{"id":"tmdb:693134"}`, 201))
	if again.ID == first.ID {
		t.Fatalf("asked again is the same ask %d", again.ID)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	hub := r.live
	before := hub.Clients()
	owner := liveSetup{}.streamWith(t, ctx, r.owner.c, r.srv.URL+"/live")
	bo := liveSetup{}.streamWith(t, ctx, r.bo.c, r.srv.URL+"/live")
	for deadline := time.Now().Add(2 * time.Second); hub.Clients() < before+2; {
		if time.Now().After(deadline) {
			t.Fatal("streams did not open")
		}
		time.Sleep(5 * time.Millisecond)
	}
	r.owner.call("POST", "/api/v1/asks/"+itoa(again.ID)+"/approve", `{}`, 200)

	type askEvent struct {
		ID           int64 `json:"id"`
		AlsoDeclined []struct {
			Name string `json:"name"`
		} `json:"also_declined"`
		DecidedBy *struct {
			Name string `json:"name"`
		} `json:"decided_by"`
		raw map[string]json.RawMessage
	}
	asksIn := func(ch <-chan liveFrame) []askEvent {
		var out []askEvent
		for _, f := range frames(ch, 800*time.Millisecond) {
			if f.topic != "ask" {
				continue
			}
			var e askEvent
			if err := json.Unmarshal([]byte(f.data), &e); err != nil {
				t.Fatalf("ask event %s: %v", f.data, err)
			}
			_ = json.Unmarshal([]byte(f.data), &e.raw)
			if e.ID == again.ID {
				out = append(out, e)
			}
		}
		return out
	}

	got := asksIn(owner)
	if len(got) == 0 {
		t.Fatal("the approver received no live ask event")
	}
	for _, e := range got {
		if len(e.AlsoDeclined) != 1 || e.AlsoDeclined[0].Name != "luis" {
			t.Fatalf("approver's live also_declined %s", e.raw["also_declined"])
		}
	}

	mine := asksIn(bo)
	if len(mine) == 0 {
		t.Fatal("the asker received no live ask event")
	}
	for _, e := range mine {
		if v, ok := e.raw["also_declined"]; ok && string(v) != "null" {
			t.Fatalf("the asker's live copy carries also_declined: %s", v)
		}
		if e.DecidedBy != nil && e.DecidedBy.Name != "" {
			t.Fatalf("the asker's live copy names the decider %q", e.DecidedBy.Name)
		}
		var st struct {
			Text string `json:"text"`
		}
		if v, ok := e.raw["step"]; ok && string(v) != "null" {
			if err := json.Unmarshal(v, &st); err != nil || st.Text != "" {
				t.Fatalf("the asker's live copy carries step text: %s", v)
			}
		}
	}
}
