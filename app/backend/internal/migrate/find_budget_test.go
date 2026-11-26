package migrate

import (
	"testing"
	"time"
)

// The worst case (nobody answers, every try waits the full findTry) must fit
// in findBudget, or the last hosts are silently never tried.
func TestFindWorstCaseFitsBudget(t *testing.T) {
	names := 0
	for _, p := range defaultPorts {
		for _, k := range p.kinds {
			names += len(dockerNames[k])
		}
	}
	hosts := 3 + 254*maxNets                         // 127.0.0.1, host.docker.internal, gateway, searched /24s
	tries := names + hosts*len(defaultPorts) + hosts // 8080 may take a second try (SABnzbd, then qBittorrent)
	worst := time.Duration((tries+findParallel-1)/findParallel) * findTry
	if worst > findBudget {
		t.Fatalf("worst case %v over budget %v (%d tries)", worst, findBudget, tries)
	}
}
