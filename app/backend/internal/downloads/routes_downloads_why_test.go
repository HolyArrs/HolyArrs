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
