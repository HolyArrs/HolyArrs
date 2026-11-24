package migrate

import (
	"strings"
	"testing"
)

// Titles whose old app folders this server cannot reach come over
// without their files; the next steps name those folders plainly.
func TestNextNamesUnreachableFolders(t *testing.T) {
	t.Parallel()
	rep := &Report{Counts: map[string]int{ResCreated: 3}, Lines: []Line{
		{Group: GroupTitles, Result: ResCreated, Attention: AttnUnreachable, Folder: "/tv"},
		{Group: GroupTitles, Result: ResCreated, Attention: AttnUnreachable, Folder: "/tv"},
		{Group: GroupTitles, Result: ResCreated, Attention: AttnUnreachable, Folder: "/movies"},
	}}
	got := strings.Join(next(rep), "\n")
	if !strings.Contains(got, "not reachable from this server") || !strings.Contains(got, "/tv, /movies") {
		t.Fatalf("next steps: %q", got)
	}
	if strings.Contains(strings.Join(next(&Report{Counts: map[string]int{}}), " "), "not reachable") {
		t.Fatal("note shown with nothing unreachable")
	}
}

// An import where no Library folder could be made (none of the old
// folders is on this server) names those folders in the next steps,
// instead of only the general path-rule line.
func TestNextNamesMissingFolders(t *testing.T) {
	t.Parallel()
	rep := &Report{Counts: map[string]int{ResNotFound: 3}, Lines: []Line{
		{Group: GroupFolders, Result: ResNotFound, missing: "/tv"},
		{Group: GroupTitles, Result: ResNotFound, missing: "/tv"},
		{Group: GroupTitles, Result: ResNotFound, missing: "/anime"},
	}}
	got := strings.Join(next(rep), "\n")
	if !strings.Contains(got, "not on this server, so nothing from them came over: /tv, /anime.") {
		t.Fatalf("next steps: %q", got)
	}
	if strings.Contains(got, "Some folders were not found") {
		t.Fatalf("general line shown although every folder is named: %q", got)
	}
}
