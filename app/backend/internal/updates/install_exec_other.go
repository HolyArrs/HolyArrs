//go:build !unix

package updates

import "errors"

// execSupported says this system can switch programs in place.
const execSupported = false

var errNoExec = errors.New("switching versions in place only works on Linux")

// execProgram is not available here: the image's own program always runs.
func execProgram(string, []string, []string) error { return errNoExec }

// Reexec is not available here.
func Reexec() error { return errNoExec }
