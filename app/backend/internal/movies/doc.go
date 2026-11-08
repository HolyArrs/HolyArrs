// Package movies holds the Movies module: movie collections, their pages
// and removing a collection. Adding and refreshing a movie stays in the
// shared title Service of the library, which records a movie's collection
// through library.UseMovies.
package movies

import "github.com/HolyArrs/holyarrs/app/backend/internal/library"

// Service runs the movie work on top of the shared title Service.
type Service struct{ *library.Service }

// New returns the movie steps for lib.
func New(lib *library.Service) *Service { return &Service{Service: lib} }

func init() { library.UseMovies(syncMovieCollectionTx) }
