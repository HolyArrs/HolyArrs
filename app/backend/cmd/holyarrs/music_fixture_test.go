package main

import (
	"fmt"
	"testing"
)

// The music fixtures the api tests that span music and other modules
// share (the music tests keep their own copy in internal/music).

// newMusicAskRig is the ask rig with a music Library folder and the fake
// MusicBrainz of the music tests.
func newMusicAskRig(t *testing.T) *askRig {
	t.Helper()
	r := newAskRig(t)
	r.asks.Library.Artists = apiMusic()
	r.owner.call("POST", "/api/v1/library-folders", fmt.Sprintf(`{"path":%q,"kind":"music","label":"Music"}`, t.TempDir()), 201)
	return r
}

func tmbid(n int) string { return fmt.Sprintf("00000000-0000-0000-0000-%012d", n) }
