package metadata

import "errors"

// ErrInstallRefused marks the metadata server refusing this install's own
// sign-in (a 401 to the hosted server): the sign-in is dropped and the
// install signs up again on its own, so a later try can work. A wrong key
// of the owner's, or a sign-in that could not be read, is never this.
var ErrInstallRefused = errors.New("the metadata server refused this install's sign-in")

type installRefused struct{ base error }

func (e installRefused) Error() string        { return e.base.Error() }
func (e installRefused) Unwrap() error        { return e.base }
func (e installRefused) Is(target error) bool { return target == ErrInstallRefused }

// InstallRefused wraps base (what callers already check for) as a refused
// install sign-in.
func InstallRefused(base error) error { return installRefused{base} }
