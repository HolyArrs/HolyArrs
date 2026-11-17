//go:build noupdater

package updates

// NoUpdater is set by the noupdater build tag for packaged installs, whose
// package manager owns updates: the app never installs a version itself.
const NoUpdater = true
