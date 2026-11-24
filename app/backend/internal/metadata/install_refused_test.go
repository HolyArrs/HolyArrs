package metadata

import (
	"errors"
	"fmt"
	"testing"
)

func TestInstallRefused(t *testing.T) {
	for _, c := range []struct {
		name              string
		err               error
		refused, noAccess bool
		serviceAccess     bool
	}{
		{"hosted TMDB 401", InstallRefused(ErrNoAccess), true, true, false},
		{"hosted service 401", fmt.Errorf("x: %w", InstallRefused(ErrServiceAccess)), true, false, true},
		{"owner's wrong key", ErrNoAccess, false, true, false},
		{"sign-in could not be read", accessErr(errors.New("vault locked")), false, false, true},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := errors.Is(c.err, ErrInstallRefused); got != c.refused {
				t.Fatalf("refused = %v", got)
			}
			if errors.Is(c.err, ErrNoAccess) != c.noAccess || errors.Is(c.err, ErrServiceAccess) != c.serviceAccess {
				t.Fatalf("base error lost: %v", c.err)
			}
		})
	}
	if InstallRefused(ErrNoAccess).Error() != ErrNoAccess.Error() {
		t.Fatal("the words changed")
	}
}
