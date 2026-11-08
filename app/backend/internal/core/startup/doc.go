// Package startup holds what the program settles before it serves: who it
// runs as (PUID, PGID, UMASK and FILE__ variables, without a shell), the
// low-memory mode and the Go memory target, and the switches for features
// held back from the first release. A held-back feature is off unless its
// name is listed in HOLYARRS_FLAGS (comma separated): its routes are not
// registered, its jobs do not run and the web app hides it. Data already saved
// for it is kept.
package startup
