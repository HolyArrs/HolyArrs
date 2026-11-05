package store

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
)

// NetworkShare reports whether dir is on a network share, and its type
// (statfs on Linux; always false elsewhere). It is the check Open refuses
// a config folder with.
func NetworkShare(dir string) (bool, string, error) { return isNetworkShare(dir) }

// OpenReadOnly opens the main database in dir for reading only, without
// migrations or the writer, so it works next to a running server. It never
// creates the file: a folder without a database is an error.
func OpenReadOnly(dir string) (*sql.DB, error) {
	path := filepath.Join(dir, MainFile)
	if _, err := os.Stat(path); err != nil {
		return nil, fmt.Errorf("no database in %s: %w", dir, err)
	}
	return openPool(path, 2000, true, 1)
}
