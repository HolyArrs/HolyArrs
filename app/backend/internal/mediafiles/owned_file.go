package mediafiles

import (
	"time"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/releasename"
)

// The functions here score a File already in the Library the way their
// Title-only forms do, and also read the release group stored on the File
// when its name names none. name is the name the File is judged by: its
// release name, or its file name when it has none. A file the old app
// renamed ("... English - HONE).mkv") names no group the parser reads, but
// the group the old app recorded for it is kept on the File at import.

// CurrentOfOwnedFile is CurrentOfTitleFile for File f (JudgeName of name,
// its size, import time and stored group).
func CurrentOfOwnedFile(f File, name string, audio []string, t Title, rules PickRulesBody) Current {
	return currentOfTitleFile(JudgeName(name, f.MediaInfo), storedGroup(f), f.Size, f.AddedAt, audio, t, rules)
}

// ScoreOwnedFile is ScoreTitleFile for File f.
func ScoreOwnedFile(f File, name string, audio []string, t Title, rules PickRulesBody) FilePoints {
	return filePoints(currentOfTitleFile(JudgeName(name, f.MediaInfo), storedGroup(f), f.Size, time.Time{}, audio, t, rules), rules)
}

// OwnedFileReachesGoodEnough is OwnedReachesGoodEnoughFor for File f.
func OwnedFileReachesGoodEnough(f File, name string, t Title, rules PickRulesBody) bool {
	return ownedReachesGoodEnough(name, f.MediaInfo, storedGroup(f), t, rules)
}

// storedGroup is the release group stored on f ("" when none).
func storedGroup(f File) string {
	if f.Group == nil {
		return ""
	}
	return *f.Group
}

// withGroup gives a file's parse the group stored on its File when the
// name names none. A group the name does name is kept.
func withGroup(p *releasename.Result, group string) {
	if p.Group.Value == "" && group != "" {
		p.Group = releasename.Field[string]{Value: group, Conf: releasename.Sure}
	}
}
