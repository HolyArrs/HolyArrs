package store

import "errors"

// ErrOwned: another process owns the storage directory (a server being
// replaced still runs); try again once it has exited.
var ErrOwned = errors.New("storage already has an owner")
