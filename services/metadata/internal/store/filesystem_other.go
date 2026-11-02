//go:build !linux && !windows

package store

import (
	"errors"
	"os"
)

func lockDirectory(string) (*os.File, error) {
	return nil, errors.New("unsupported storage operating system")
}

func localSpace(string) (uint64, error) { return 0, errors.New("unsupported storage operating system") }
