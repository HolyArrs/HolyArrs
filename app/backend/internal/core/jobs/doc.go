// Package jobs runs background work. Its queue keeps jobs and their schedules
// in the database and runs them in pools with retries, and its supervisor
// keeps the program's long-running loops alive and reports their state for
// the health page.
package jobs
