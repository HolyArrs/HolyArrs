// Package archive unpacks the archive kinds other than RAR that a release may
// come in: 7-Zip (one file or .7z.001 parts), zip, tar (plain, gzip, bzip2 or
// xz) and a single gzip, bzip2 or xz file. The files are untrusted, so it uses
// the same rules as package unrar: names pass unrar.CleanName, links and
// special entries are refused, every file is written through a handle on the
// target folder and never over an existing one, the output is capped, reading
// stops when the context ends, and a password-protected archive is refused.
// It returns the unrar errors so the importer gives the same plain reasons.
package archive
