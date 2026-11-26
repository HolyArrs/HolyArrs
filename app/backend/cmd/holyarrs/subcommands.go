package main

import (
	"os"

	"github.com/HolyArrs/holyarrs/app/backend/internal/updates"
)

// subcommand runs a command given in place of the server (reset-owner,
// restore, import-secrets, the update check, doctor) and reports its exit
// code; ok is false for a plain server start.
func subcommand(args []string) (code int, ok bool) {
	if len(args) == 0 {
		return 0, false
	}
	switch args[0] {
	case "reset-owner":
		return resetOwner(args[1:]), true
	case "restore":
		return restoreCmd(args[1:], os.Stdout, os.Stderr), true
	case "import-secrets":
		return importSecrets(args[1:], os.Stdout, os.Stderr), true
	case updates.CheckCommand:
		return updateCheckCmd(args[1:], os.Stderr), true
	case "doctor":
		return doctorCmd(args[1:], os.Stdout, os.Stderr), true
	}
	return 0, false
}
