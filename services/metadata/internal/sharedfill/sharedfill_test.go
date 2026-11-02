package sharedfill

import (
	"context"
	"testing"
)

func TestOnceQueuesEachMemberOnce(t *testing.T) {
	var got []Member
	f := &Filler{
		Lists: map[string]func(context.Context) []Member{
			"charts":    func(context.Context) []Member { return []Member{{"artist", "a"}, {"release-group", "r"}} },
			"genre:pop": func(context.Context) []Member { return []Member{{"artist", "a"}, {"artist", "b"}, {"artist", ""}} },
		},
		Enqueue: func(_ context.Context, m Member) { got = append(got, m) },
	}
	p := f.Once(context.Background())
	if len(got) != 3 || p.Members != 3 || p.Lists["genre:pop"] != 3 {
		t.Fatalf("queued %v pass %+v", got, p)
	}
	if !f.Listed("artist", "b") || f.Listed("artist", "z") {
		t.Fatal("listed set wrong")
	}
}

func TestOnceQueuesOneHopOfRelated(t *testing.T) {
	var got []Member
	var asked []Member
	f := &Filler{
		Lists: map[string]func(context.Context) []Member{
			"charts": func(context.Context) []Member { return []Member{{"artist", "a"}, {"release-group", "r"}} },
		},
		Related: func(_ context.Context, m Member) []Member {
			asked = append(asked, m)
			return []Member{{"artist", "a"}, {"artist", "x"}, {"artist", "x"}}
		},
		Enqueue: func(_ context.Context, m Member) { got = append(got, m) },
	}
	p := f.Once(context.Background())
	// a, r from the list; x once as related. Related is asked only for
	// listed artists, so x's own similar artists are not followed.
	if len(got) != 3 || p.Related != 1 || len(asked) != 1 || asked[0].MBID != "a" {
		t.Fatalf("queued %v asked %v pass %+v", got, asked, p)
	}
}

func TestMembersReadableWhileRelatedRuns(t *testing.T) {
	var f *Filler
	seenDuring := 0
	f = &Filler{
		Lists:   map[string]func(context.Context) []Member{"a": func(context.Context) []Member { return []Member{{"artist", "x"}} }},
		Enqueue: func(context.Context, Member) {},
		Related: func(context.Context, Member) []Member {
			seenDuring = len(f.Members())
			return nil
		},
	}
	f.Once(context.Background())
	if seenDuring != 1 {
		t.Fatalf("members during related hop = %d, want 1", seenDuring)
	}
}
