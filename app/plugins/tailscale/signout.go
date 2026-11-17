package main

import (
	"context"
	"os"
	"path/filepath"
)

// stateDir is where tsnet keeps its keys and certificates inside the data folder.
func stateDir(data string) string { return filepath.Join(data, "tsnet") }

// signOut logs the device out when it can, then wipes the state folder
// either way. logoutErr tells the caller to say "remove it in Tailscale's
// admin page" when the logout itself failed.
func signOut(ctx context.Context, data string, logout func(context.Context) error) (logoutErr, wipeErr error) {
	if logout != nil {
		logoutErr = logout(ctx)
	}
	wipeErr = os.RemoveAll(stateDir(data))
	if wipeErr == nil {
		wipeErr = writeStatus(data, Status{State: StateNotConnected, Updated: now()})
	}
	return logoutErr, wipeErr
}

// exitLoggedOutLocally is the --sign-out exit code when the state was wiped
// but the logout itself failed, so HolyArrs can tell the owner to remove the
// device in Tailscale's admin page.
const exitLoggedOutLocally = 3

// signOutCode is the exit code for a sign-out whose wipe worked.
func signOutCode(logoutErr error) int {
	if logoutErr != nil {
		return exitLoggedOutLocally
	}
	return 0
}
