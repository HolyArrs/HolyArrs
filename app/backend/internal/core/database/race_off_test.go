//go:build !race

package database

// raceOn says the race detector is on: it slows every write several times.
const raceOn = false
