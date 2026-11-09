package main

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/discover"
	"github.com/HolyArrs/holyarrs/app/backend/internal/mediafiles"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/webserver"
)

// Episode and track lists leave out the same Slot details as a title does.
func TestEpisodeSlotsTrimmedForMember(t *testing.T) {
	t.Parallel()
	mk := func() map[int64][]EpisodeSlot {
		f := mediafiles.File{RelativePath: "Shows/Secret/S01E01.mkv", Size: 9}
		return map[int64][]EpisodeSlot{1: {{Reason: "turned down: Secret.Release-GRP", File: &f}}}
	}
	member := webserver.ContextWith(context.Background(), webserver.Caller{UserID: 2, Role: webserver.RoleMember, Perms: []webserver.Perm{webserver.PermAsk}, Via: "session"})
	got := mk()
	discover.TrimEpisodeSlots(member, got)
	if s := got[1][0]; s.Reason != "" || s.File.RelativePath != "" || s.File.Size != 9 {
		t.Fatalf("member slot %+v %+v", s, s.File)
	}
	manager := webserver.ContextWith(context.Background(), webserver.Caller{UserID: 3, Role: webserver.RoleMember, Perms: []webserver.Perm{webserver.PermManageLibrary}, Via: "session"})
	kept := mk()
	discover.TrimEpisodeSlots(manager, kept)
	if s := kept[1][0]; s.Reason == "" || s.File.RelativePath == "" {
		t.Fatalf("manager slot trimmed %+v", s)
	}
}
