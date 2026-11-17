package downloads

import "testing"

// A finished torrent counts as seeding only when its client still shares it.
func TestClientPhaseSeedingPerClient(t *testing.T) {
	cases := []struct {
		detail string
		want   string
	}{
		{"uploading", "seeding"}, {"stalledUP", "seeding"}, {"forcedUP", "seeding"},
		{"pausedUP", "paused"}, {"stoppedUP", "paused"},
		{"6", "seeding"}, {"5", "seeding"}, {"0", "paused"}, {"6; error 1: tracker", "seeding"},
		{"Seeding", "seeding"}, {"Paused", "paused"},
		{"seeding", "seeding"}, {"complete", "finished"},
	}
	for _, c := range cases {
		_, st, _ := clientPhase(Job{State: StateFinished, Detail: c.detail})
		if st != c.want {
			t.Errorf("detail %q: state %q, want %q", c.detail, st, c.want)
		}
	}
}
