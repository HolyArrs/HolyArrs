// Package jobs runs background work. Its queue keeps jobs and their schedules
// in the database and runs them in pools with retries, and its supervisor
// keeps the program's long-running loops alive and reports their state for
// the health page.
//
// The supervisor starts every background loop the same way: a crash is
// caught and logged, the stage is marked failed, and it starts again after a
// growing, capped delay. A pass that ignores its deadline is shown stuck and
// never started a second time. The Registry also keeps the state of every
// linked app, so the health page reads one list.
package jobs
