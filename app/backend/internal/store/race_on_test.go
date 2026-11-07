//go:build race

package store

// raceOn says the race detector is on: it slows every write several times.
const raceOn = true
