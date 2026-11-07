package api

import (
	"context"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/auth"
	"github.com/HolyArrs/holyarrs/app/backend/internal/domain"
)

// Episode and track lists leave out the same Slot details as a title does.
func TestEpisodeSlotsTrimmedForMember(t *testing.T) {
	mk := func() map[int64][]EpisodeSlot {
		f := domain.File{RelativePath: "Shows/Secret/S01E01.mkv", Size: 9}
		return map[int64][]EpisodeSlot{1: {{Reason: "turned down: Secret.Release-GRP", File: &f}}}
	}
	member := auth.ContextWith(context.Background(), auth.Who{UserID: 2, Role: auth.RoleMember, Perms: []auth.Perm{auth.PermAsk}, Via: "session"})
	got := mk()
	trimEpisodeSlots(member, got)
	if s := got[1][0]; s.Reason != "" || s.File.RelativePath != "" || s.File.Size != 9 {
		t.Fatalf("member slot %+v %+v", s, s.File)
	}
	manager := auth.ContextWith(context.Background(), auth.Who{UserID: 3, Role: auth.RoleMember, Perms: []auth.Perm{auth.PermManageLibrary}, Via: "session"})
	kept := mk()
	trimEpisodeSlots(manager, kept)
	if s := kept[1][0]; s.Reason == "" || s.File.RelativePath == "" {
		t.Fatalf("manager slot trimmed %+v", s)
	}
}
