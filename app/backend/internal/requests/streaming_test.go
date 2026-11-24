package requests

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	domain "github.com/HolyArrs/holyarrs/app/backend/internal/core/model"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

var netflix, disney = metadata.WatchService{ID: 8, Name: "Netflix"}, metadata.WatchService{ID: 337, Name: "Disney Plus"}

func TestOnMyServices(t *testing.T) {
	got := map[string][]metadata.WatchService{"US": {disney, netflix}, "DE": {netflix}}
	for _, c := range []struct {
		name     string
		region   string
		services []int64
		want     string
	}{
		{"first of the owner's order wins", "US", []int64{8, 337}, "Netflix"},
		{"other order", "US", []int64{337, 8}, "Disney Plus"},
		{"lower case region", "us", []int64{337}, "Disney Plus"},
		{"not in that country", "DE", []int64{337}, ""},
		{"country not listed", "FR", []int64{8}, ""},
		{"no services", "US", nil, ""},
		{"empty region", "", []int64{8}, ""},
	} {
		sv, ok := OnMyServices(got, c.region, c.services)
		if (c.want != "") != ok || sv.Name != c.want {
			t.Errorf("%s: got %q %v, want %q", c.name, sv.Name, ok, c.want)
		}
	}
	if _, ok := OnMyServices(nil, "US", []int64{8}); ok {
		t.Error("nil answer matched")
	}
}

func TestCheckStreaming(t *testing.T) {
	many := make([]int64, maxMyServices+1)
	for i := range many {
		many[i] = int64(i + 1)
	}
	for _, c := range []struct {
		name     string
		in       Settings
		err      error
		region   string
		services []int64
	}{
		{"empty is fine", Settings{}, nil, "", []int64{}},
		{"tidied", Settings{StreamingRegion: " us ", StreamingServices: []int64{8, 8, 337}}, nil, "US", []int64{8, 337}},
		{"three letters", Settings{StreamingRegion: "USA"}, ErrStreamingRegion, "", nil},
		{"digits", Settings{StreamingRegion: "U1"}, ErrStreamingRegion, "", nil},
		{"zero id", Settings{StreamingServices: []int64{0}}, ErrStreamingServices, "", nil},
		{"negative id", Settings{StreamingServices: []int64{-4}}, ErrStreamingServices, "", nil},
		{"too many", Settings{StreamingServices: many}, ErrStreamingServices, "", nil},
	} {
		s := c.in
		err := checkStreaming(&s)
		if !errors.Is(err, c.err) {
			t.Errorf("%s: err %v, want %v", c.name, err, c.err)
			continue
		}
		if err == nil && (s.StreamingRegion != c.region || !reflect.DeepEqual(s.StreamingServices, c.services)) {
			t.Errorf("%s: got %q %v", c.name, s.StreamingRegion, s.StreamingServices)
		}
	}
}

func TestParseServices(t *testing.T) {
	for in, want := range map[string][]int64{"": {}, "8": {8}, "8|337": {8, 337}, "8|x|-1|0|8| 9 ": {8, 9}} {
		if got := parseServices(in); !reflect.DeepEqual(got, want) {
			t.Errorf("%q: %v, want %v", in, got, want)
		}
	}
}

func TestStreamingSettingsSave(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	s, _ := ReadSettings(ctx, r.st.Read())
	if s.SkipOnServices || s.StreamingRegion != "" || len(s.StreamingServices) != 0 {
		t.Fatalf("defaults %+v", s)
	}
	s.SkipOnServices, s.StreamingRegion, s.StreamingServices = true, "gb", []int64{8, 337}
	if err := SaveSettings(ctx, r.st, s, domain.System); err != nil {
		t.Fatal(err)
	}
	s, _ = ReadSettings(ctx, r.st.Read())
	if !s.SkipOnServices || s.StreamingRegion != "GB" || !reflect.DeepEqual(s.StreamingServices, []int64{8, 337}) {
		t.Fatalf("saved %+v", s)
	}
	s.StreamingRegion = "GBR"
	if err := SaveSettings(ctx, r.st, s, domain.System); !errors.Is(err, ErrStreamingRegion) {
		t.Fatalf("bad region saved: %v", err)
	}
}

func TestSkipWhenOnMyServices(t *testing.T) {
	type answer struct {
		got map[string][]metadata.WatchService
		err error
	}
	onNetflix := answer{got: map[string][]metadata.WatchService{"US": {netflix}}}
	for _, c := range []struct {
		name   string
		skip   bool
		region string
		answer answer
		want   string
	}{
		{"on a service: turned down", true, "US", onNetflix, domain.AskTurnedDown},
		{"switch off: downloaded", false, "US", onNetflix, domain.AskApproved},
		{"other country: downloaded", true, "DE", onNetflix, domain.AskApproved},
		{"nowhere: downloaded", true, "US", answer{got: map[string][]metadata.WatchService{}}, domain.AskApproved},
		{"metadata down: downloaded", true, "US", answer{err: errors.New("down")}, domain.AskApproved},
	} {
		t.Run(c.name, func(t *testing.T) {
			r := newRig(t)
			ctx := context.Background()
			calls := 0
			r.svc.Watch = func(context.Context, string, string) (map[string][]metadata.WatchService, error) {
				calls++
				return c.answer.got, c.answer.err
			}
			s, _ := ReadSettings(ctx, r.st.Read())
			s.SkipOnServices, s.StreamingRegion, s.StreamingServices = c.skip, c.region, []int64{8}
			if err := SaveSettings(ctx, r.st, s, domain.System); err != nil {
				t.Fatal(err)
			}
			res, err := r.svc.Ask(ctx, r.ana, "693134")
			if err != nil {
				t.Fatal(err)
			}
			a, said, err := r.svc.ApproveSaying(ctx, res.Ask.ID, r.luis, ApproveOptions{})
			if err != nil || a.State != c.want {
				t.Fatalf("state %s %v, want %s", a.State, err, c.want)
			}
			if c.want == domain.AskTurnedDown {
				if !strings.Contains(said, "Netflix") || !strings.Contains(a.Note, "Netflix") {
					t.Fatalf("said %q note %q", said, a.Note)
				}
				// Approving it again downloads it anyway, without asking again.
				before := calls
				a, said, err = r.svc.ApproveSaying(ctx, a.ID, r.luis, ApproveOptions{})
				if err != nil || a.State != domain.AskApproved || said != "" || calls != before {
					t.Fatalf("again %s %q %v calls %d", a.State, said, err, calls-before)
				}
			}
		})
	}
}

func TestSkipOnAutoApprove(t *testing.T) {
	r := newRig(t)
	ctx := context.Background()
	r.svc.Watch = func(context.Context, string, string) (map[string][]metadata.WatchService, error) {
		return map[string][]metadata.WatchService{"US": {netflix}}, nil
	}
	s, _ := ReadSettings(ctx, r.st.Read())
	s.SkipOnServices, s.StreamingRegion, s.StreamingServices = true, "US", []int64{8}
	_ = SaveSettings(ctx, r.st, s, domain.System)
	res, err := r.svc.Ask(ctx, r.luis, "693134")
	if err != nil || res.Ask.State != domain.AskTurnedDown {
		t.Fatalf("auto ask %+v %v", res.Ask, err)
	}
	if res.Ask.TitleID != nil {
		t.Fatal("title added for a skipped ask")
	}
}
