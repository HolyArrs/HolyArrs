package plugins

import (
	"slices"
)

// ReachHosts are the hosts id may connect to: those the owner approved
// from its manifest plus those the owner added, sorted. The net lock lets
// it reach nothing else, so a change here is a change of where what it is
// handed can go.
func (e *Engine) ReachHosts(id string) []string {
	if e.init() != nil {
		return nil
	}
	p, err := e.installed(id)
	if err != nil {
		return nil
	}
	a := approvalFor(p, e.readManifest(id))
	out := append(slices.Clone(a.Hosts), a.OwnerHosts...)
	slices.Sort(out)
	return slices.Compact(out)
}
