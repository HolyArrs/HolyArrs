package main

import (
	"errors"
	"fmt"
)

// errRestart asks main to exit so the container starts HolyArrs again,
// which puts a staged restore in place.
var errRestart = errors.New("restart requested")

// exitRestart is the exit status after a staged restore and after a
// restart asked for from the controls (not zero, so a restart policy of
// on-failure starts the app again too).
const exitRestart = 75

// errAppExit ends serve because a person asked for a restart or a shut
// down from the controls; main exits with code.
type errAppExit struct{ code int }

func (e errAppExit) Error() string { return fmt.Sprintf("exit %d asked for from the controls", e.code) }

// safeNextFile in the config folder makes the next start a safe-mode start
// (restart in safe mode); it is removed as that start reads it.
const safeNextFile = "safe-mode-next"
