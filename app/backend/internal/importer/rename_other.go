//go:build !linux && !darwin && !windows

package importer

// renameNoReplace renames src to dst and never replaces an existing dst.
func renameNoReplace(src, dst string) error { return renameChecked(src, dst) }
