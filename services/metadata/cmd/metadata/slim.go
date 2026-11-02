package main

import (
	"context"
	"log"
	"strings"

	"github.com/CoderLuii/holyarrs-metadata/internal/api"
	"github.com/CoderLuii/holyarrs-metadata/internal/mbread"
)

// SlimRootEnv names the slim MusicBrainz copy's root (the folder holding
// CURRENT and the dated copies); it overrides mb_slim_root in the config.
const SlimRootEnv = "HOLYARRS_MB_SLIM_ROOT"

// openSlim opens the slim MusicBrainz dump copy when a root is set, and
// sends MusicBrainz detail lookups to it first: pages, stored-answer
// readers and the discography store read the copy, and the live queue
// runs only for items it lacks. Unset: nothing changes.
func openSlim(c config, getenv func(string) string, o *api.Options) (*mbread.Reader, error) {
	root := slimRoot(c, getenv)
	if root == "" {
		return nil, nil
	}
	r, err := mbread.Open(root)
	if err != nil {
		return nil, err
	}
	o.Slim = r.Stored
	o.SlimMissing = func(ctx context.Context, kind, mbid string) bool { return slimMissing(ctx, r, kind, mbid) }
	log.Printf("slim MusicBrainz copy: root %s, dump %q", root, r.Date())
	return r, nil
}

// slimRoot answers the slim copy's root ("" when unset).
func slimRoot(c config, getenv func(string) string) string {
	if root := strings.TrimSpace(getenv(SlimRootEnv)); root != "" {
		return root
	}
	return strings.TrimSpace(c.MBSlimRoot)
}

// slimHolder is the part of the slim reader slimMissing reads.
type slimHolder interface {
	Date() string
	Has(ctx context.Context, kind, mbid string) bool
}

// slimMissing reports an id the loaded copy does not hold; false with no
// copy loaded or for a kind the copy does not carry.
func slimMissing(ctx context.Context, r slimHolder, kind, mbid string) bool {
	if r == nil || r.Date() == "" {
		return false
	}
	switch kind {
	case "artist", "release-group", "release", "recording":
		return !r.Has(ctx, kind, mbid)
	}
	return false
}
