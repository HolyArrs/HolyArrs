// Package tags reads the few tags of a music file that help match it to a
// track: title, artist, album, and track and disc numbers. It reads ID3v2 tags
// at the start of an MP3 file and the Vorbis comments of a FLAC file, and never
// writes. Every size read from the file is checked against hard limits, so a
// broken or hostile file costs at most a few hundred kilobytes of reading and
// gives no tags.
package tags
