//go:build !labkeys

package main

// labPluginKeys is empty in normal and release builds: only the pinned
// keys are trusted, whatever the environment says.
func labPluginKeys() []string { return nil }
