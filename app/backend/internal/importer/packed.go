package importer

import (
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer/archive"
	"github.com/HolyArrs/holyarrs/app/backend/internal/importer/tags"
)

// ArchiveKind says whether name is a packed file the import can unpack
// (its kind, or "" when it is not), and whether it is the first part of a
// set.
func ArchiveKind(name string) (kind string, first bool) {
	k, f := archive.Of(name)
	return string(k), f
}

// FileTags are the tags read from one music file.
type FileTags = tags.Tags

// ReadFileTags reads the tags of the music file at path.
func ReadFileTags(path string) (FileTags, error) { return tags.ReadFile(path) }
