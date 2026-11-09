// Package migrate moves an owner's setup over from the apps they are
// leaving, so they do not have to rebuild it by hand. Each old app is read
// into a Snapshot of plain data, either from its API or from a backup file,
// and no secret from it is kept in the main database. A plan is built from
// the Snapshots and shown as a dry run first, then the move runs as a job
// that can be undone, because the owner must be able to back out safely.
// Backups are opened read only on a copy with size limits, since they come
// from outside and may be broken or hostile.
package migrate
