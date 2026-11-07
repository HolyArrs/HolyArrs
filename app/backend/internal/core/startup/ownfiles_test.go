package startup

import "testing"

// Files copied into /config by root (docker cp into a named volume, sudo cp,
// a root backup tool) keep root as owner while the top folder is already
// right: the walk must still run when any of HolyArrs's own files is wrong.
func TestOwnFilesWrongTriggerTheWalk(t *testing.T) {
	owners := map[string][2]int{}
	owner := func(name string) (int, int, bool) {
		o, ok := owners[name]
		return o[0], o[1], ok
	}
	if OwnFilesNeedFix(owner, 1000, 1000) {
		t.Fatal("no files yet (first start): nothing to fix")
	}
	owners["holyarrs.db"] = [2]int{1000, 1000}
	owners["logs"] = [2]int{1000, 1000}
	if OwnFilesNeedFix(owner, 1000, 1000) {
		t.Fatal("all owned right: no walk")
	}
	for _, name := range []string{"holyarrs.db", "holyarrs.db-wal", "holyarrs.db-shm", "holyarrs-events.db", "holyarrs.key", "logs", "logs/holyarrs.log"} {
		owners[name] = [2]int{0, 0}
		if !OwnFilesNeedFix(owner, 1000, 1000) {
			t.Errorf("%s owned by root: should walk", name)
		}
		owners[name] = [2]int{1000, 1000}
	}
	owners["holyarrs.key"] = [2]int{1000, 0}
	if !OwnFilesNeedFix(owner, 1000, 1000) {
		t.Error("wrong group only: should walk")
	}
}
