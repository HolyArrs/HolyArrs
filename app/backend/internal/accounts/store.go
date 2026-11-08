package accounts

import "github.com/HolyArrs/holyarrs/app/backend/internal/core/database"

// conn is a pool or a transaction: the store functions of this package run
// on whichever the caller holds, so a handler's statements stay in its one
// write transaction.
type conn interface {
	database.Querier
	database.Lister
	database.Execer
}
