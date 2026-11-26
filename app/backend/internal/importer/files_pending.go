package importer

import (
	"errors"
	"io/fs"
	"os"
	"strings"
)

// NotPresentYetWhy is an expected file in a visible download folder that has
// not reached this host. This differs from a file that vanished during import.
const NotPresentYetWhy = "the file is not present yet; HolyArrs will try importing it again"

// CopyingWhy is a temporary import block: the client finished downloading,
// but the local copy has not reached the size in its file list yet.
const CopyingWhy = "the downloaded file is still being copied; HolyArrs will try importing it again"

// FilesPending includes the reasons older imports stored when only the
// shared parent folder existed. That does not prove the file was removed:
// a seedbox sync may not have copied this particular download here yet.
func FilesPending(reason string) bool {
	return reason == notThereWhy || reason == NotPresentYetWhy || reason == CopyingWhy || reason == fileTakenWhy ||
		strings.HasPrefix(reason, "the file for ") && strings.HasSuffix(reason, " could not be placed: "+notInFolderWhy)
}

// sourceNotPresent distinguishes a missing listed file from one that vanished
// during placement. Only use this when the download root itself is available.
func sourceNotPresent(root string, cands []Candidate, file string) bool {
	fi, err := os.Stat(root)
	if err != nil || !fi.IsDir() {
		return false
	}
	for _, c := range cands {
		if c.Path == file {
			_, err := os.Stat(file)
			return errors.Is(err, fs.ErrNotExist)
		}
	}
	return false
}

// copyInProgress compares a present file with its expected size before
// inspecting or placing it, so a partial sync is neither imported nor called a fake.
func copyInProgress(cands []Candidate, file string) bool {
	for _, c := range cands {
		if c.Path == file && c.Size > 0 {
			fi, err := os.Stat(file)
			return err == nil && fi.Mode().IsRegular() && fi.Size() > 0 && fi.Size() < c.Size
		}
	}
	return false
}

// FolderMissing is a whole download folder not found here: often a wrong
// path pair or mount, so the screen keeps that hint while it retries.
func FolderMissing(reason string) bool { return reason == notThereWhy }
