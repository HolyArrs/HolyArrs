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

// A file that matched but could not be placed waits for a person with its own
// plain line, never as "ran out of Releases".
func TestWhyCountsPlacingReasonsAsAStuckImport(t *testing.T) {
	t.Parallel()
	for _, r := range []string{
		"a file with the same name that HolyArrs did not place is already in the folder; move it out of the folder, then import again",
		"HolyArrs may not write to the folder this file goes to in the Library; give the user HolyArrs runs as (PUID and PGID) write access to it, then import again",
		"the file could not be placed: hardlink failed (x), copy failed (y)",
		"HolyArrs can't find the downloaded file where the Downloader says it is; check the path pair in Settings > Downloaders and that the download folder is mounted, then import again",
	} {
		if got := why(r); got != "conflict" {
			t.Errorf("%q: %s, want conflict", r, got)
		}
	}
}

// A movie whose file is gone while its download folder is still there is an
// import that could not finish, as Sonarr and Radarr show it: never blamed on
// another app, in today's words and the system's words older imports left.
func TestWhyCountsAMovieFileGoneFromItsFolderAsStuck(t *testing.T) {
	t.Parallel()
	for _, r := range []string{
		"the file could not be placed: the file is not in the download folder any more",
		"the file could not be placed: lstat /data/downloads/x.mkv: no such file or directory",
	} {
		if got := why(r); got != "conflict" {
			t.Errorf("%q: %s, want conflict", r, got)
		}
	}
}

// An episode whose file is missing from the download is an import that could
// not finish, like a Library folder that is not there.
func TestWhyMissingEpisodeFileIsStuck(t *testing.T) {
	t.Parallel()
	for _, r := range []string{
		"the file could not be placed: lstat /mnt/Torrents/holyarrs/Show.S01/Show.S01E01.mkv: no such file or directory",
		"the file could not be placed: CreateFile D:/dl/Show.S01E01.mkv: The system cannot find the file specified.",
		"the file for S01E01 could not be placed: Copper Valley by clean name; holds S01E01; the file is not in the download folder any more",
	} {
		if got := why(r); got != "conflict" {
			t.Errorf("%q: %s, want conflict", r, got)
		}
	}
	// A Library folder that is not there, or a placing that failed with the file still in the download, is a stuck import.
	for _, r := range []string{
		"the file could not be placed: hardlink failed (x), copy failed (y)",
		`the file could not be placed: mkdir Z:\Movies\Dune (2021): The system cannot find the path specified.`,
		"the file could not be placed: hardlink failed (link /dl/x.mkv /movies/x.mkv: no such file or directory), copy failed (open /movies/.holyarrs-1.part: no such file or directory)",
	} {
		if got := why(r); got != "conflict" {
			t.Errorf("%q: %s, want conflict", r, got)
		}
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
