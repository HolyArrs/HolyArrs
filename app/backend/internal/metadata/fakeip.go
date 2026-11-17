package metadata

import (
	"errors"
	"net/url"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/outbound"
)

// fakeIPDown is ErrServiceUnavailable for a server name that resolved into
// the fake-IP range, with words that name the cause and the host.
type fakeIPDown struct{ host string }

func (e fakeIPDown) Error() string   { return outbound.FakeIPMessage(e.host) }
func (e fakeIPDown) Is(t error) bool { return t == ErrServiceUnavailable }

// FakeIPHost is the host a fake-address refusal in err names, or "".
func FakeIPHost(err error) string {
	var d fakeIPDown
	if errors.As(err, &d) {
		return d.host
	}
	var fe *outbound.FakeIPError
	if errors.As(err, &fe) {
		return fe.Host
	}
	return ""
}

// ServerFakeIP is the metadata server's host when its name was lately sent
// to a fake address, else "".
func ServerFakeIP() string {
	u, err := url.Parse(ServiceURL())
	if err != nil || !outbound.FakeIPSeen(u.Hostname()) {
		return ""
	}
	return u.Hostname()
}
