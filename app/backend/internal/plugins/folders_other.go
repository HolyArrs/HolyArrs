//go:build !linux

package plugins

import (
	"errors"
	"os"
)

// folderID is not needed off Linux, where plugins never run.
func folderID(string) (uint64, uint64, error) { return 0, 0, nil }

func openFolder(string, uint64, uint64) (*os.File, error) {
	return nil, errors.New("plugins run only on Linux")
}
