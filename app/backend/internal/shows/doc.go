// Package shows holds the Shows module: adding a show, its seasons and
// episodes, the show refresh and schedule, and adopting show folders.
// Service wraps the shared title Service of the library; the library calls
// back into it for show Titles through library.ShowParts.
package shows

import "github.com/HolyArrs/holyarrs/app/backend/internal/library"

// Service runs the show work on top of the shared title Service.
type Service struct{ *library.Service }

// New returns the show steps for lib.
func New(lib *library.Service) *Service { return &Service{Service: lib} }
