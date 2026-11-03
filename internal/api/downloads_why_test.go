package api

import "testing"

// An unpack that falls short (#54) is a stuck import on the Needs a decision
// card, not "ran out of Releases".
func TestWhyCountsUnpackReasonsAsAStuckImport(t *testing.T) {
	for _, r := range []string{
		"Packed in password-protected RAR files; they cannot be unpacked",
		"The RAR files look damaged or incomplete, so they could not be unpacked",
		"A RAR file holds a path that is not safe, so it was not unpacked",
		"Not enough free space to unpack the RAR files next to the download",
		"The RAR files could not be unpacked safely",
		"The RAR files would unpack to more than is allowed",
		"Packed in a RAR format from before RAR 1.5, which HolyArrs cannot unpack",
		"Packed in .7z or .zip files; HolyArrs unpacks only RAR files",
		"The disk filled up while the RAR files were being unpacked; nothing was imported",
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
