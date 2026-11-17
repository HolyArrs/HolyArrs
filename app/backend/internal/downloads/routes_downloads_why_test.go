package downloads

import "testing"

// An unpack that falls short is a stuck import on the Needs a decision
// card, not "ran out of Releases".
func TestWhyCountsUnpackReasonsAsAStuckImport(t *testing.T) {
	t.Parallel()
	for _, r := range []string{
		"Packed in password-protected archive files; they cannot be unpacked",
		"The archive files look damaged or incomplete, so they could not be unpacked",
		"An archive file holds a path that is not safe, so it was not unpacked",
		"Not enough free space to unpack the archive files next to the download",
		"The archive files could not be unpacked safely",
		"The archive files would unpack to more than is allowed",
		"Packed in a RAR format from before RAR 1.5, which HolyArrs cannot unpack",
		"The disk filled up while the archive files were being unpacked; nothing was imported",
	} {
		if got := why(r); got != "conflict" {
			t.Errorf("%q: %s, want conflict", r, got)
		}
	}
	if got := why("not sent: no torrent Downloader has this title's tags (kids); add one of them to a Downloader, or take them off the title"); got != "not_sent" {
		t.Errorf("tags: %s", got)
	}
	if got := why("no other Release passed the Pick rules"); got != "ran_out" {
		t.Errorf("ran out: %s", got)
	}
}

// An episode parked by an older import whose files another app had already
// taken says so, so the card does not offer a second download first.
func TestWhyNamesFilesTakenByAnotherApp(t *testing.T) {
	t.Parallel()
	for _, r := range []string{
		"the file could not be placed: lstat /mnt/Torrents/holyarrs/Show.S01/Show.S01E01.mkv: no such file or directory",
		"the file could not be placed: CreateFile D:/dl/Show.S01E01.mkv: The system cannot find the file specified.",
		"the file for S01E01 could not be placed: Copper Valley by clean name; holds S01E01; the file is not in the download folder any more",
	} {
		if got := why(r); got != "taken" {
			t.Errorf("%q: %s, want taken", r, got)
		}
	}
	if got := why("the file could not be placed: hardlink failed (x), copy failed (y)"); got != "conflict" {
		t.Errorf("other placing error: %s", got)
	}
}

// A file the importer would not put in place of the copy, or whose edition it could not settle, is a stuck import, not "ran out of Releases".
func TestWhyCountsCopyRefusalsAsAStuckImport(t *testing.T) {
	t.Parallel()
	for _, r := range []string{
		"the copy in place is protected; choose another Version for an upgrade",
		"the new edition differs from the copy in place; choose another Version",
		"edition is unknown; confirm the edition or choose an unconstrained Version",
	} {
		if got := why(r); got != "conflict" {
			t.Errorf("%q: %s, want conflict", r, got)
		}
	}
}
