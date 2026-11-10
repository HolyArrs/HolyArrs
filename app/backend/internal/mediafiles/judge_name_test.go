package mediafiles

import "testing"

func TestJudgeNameAddsInspectedResolution(t *testing.T) {
	info := map[string]any{"resolution": "1080p"}
	if got := JudgeName("The Rookie - S01E02 - Title", info); got != "The Rookie - S01E02 - Title 1080p" {
		t.Fatalf("no resolution in the name: got %q", got)
	}
	if got := JudgeName("The Matrix (1999) Bluray-720p", info); got != "The Matrix (1999) Bluray-720p" {
		t.Fatalf("the name's own resolution wins: got %q", got)
	}
	if got := JudgeName("Show - S01E02 - Title", nil); got != "Show - S01E02 - Title" {
		t.Fatalf("no inspection: got %q", got)
	}
	if got := JudgeName("Show - S01E02 - Title", map[string]any{"resolution": "odd"}); got != "Show - S01E02 - Title" {
		t.Fatalf("odd inspection value: got %q", got)
	}
}

func TestOwnedFileWithoutSourceGetsTheBenefitOfTheDoubt(t *testing.T) {
	rules := GoodEveryday()
	at1080 := map[string]any{"resolution": "1080p"}
	at720 := map[string]any{"resolution": "720p"}
	if !OwnedReachesGoodEnough("The Rookie - S01E02 - Crash Course", at1080, rules) {
		t.Fatal("a 1080p episode with no source in its name should count as good enough")
	}
	if OwnedReachesGoodEnough("The Rookie - S01E02 - Crash Course", at720, rules) {
		t.Fatal("a 720p episode is below a 1080p good-enough step")
	}
	if OwnedReachesGoodEnough("The Matrix (1999) HDTV-1080p", at1080, rules) {
		t.Fatal("a file whose name names its source is judged as named")
	}
	if !OwnedReachesGoodEnough("The Matrix (1999) WEBDL-1080p", at1080, rules) {
		t.Fatal("a named web 1080p file reaches good enough")
	}
}

func TestJudgeNameUsesTheRecordedQuality(t *testing.T) {
	info := map[string]any{"resolution": "1080p", RecordedQualityKey: "WEBDL-1080p"}
	if got := JudgeName("The Rookie - S01E01 - Pilot", info); got != "The Rookie - S01E01 - Pilot WEBDL-1080p" {
		t.Fatalf("got %q", got)
	}
	rules := GoodEveryday()
	if !OwnedReachesGoodEnough("The Rookie - S01E01 - Pilot", info, rules) {
		t.Fatal("a recorded web 1080p episode reaches good enough")
	}
	tv := map[string]any{"resolution": "1080p", RecordedQualityKey: "HDTV-1080p"}
	if OwnedReachesGoodEnough("The Rookie - S01E01 - Pilot", tv, rules) {
		t.Fatal("a recorded TV 1080p episode is below a web 1080p good-enough step")
	}
	if got := JudgeName("The Matrix (1999) Bluray-1080p", info); got != "The Matrix (1999) Bluray-1080p" {
		t.Fatalf("a name with its own source keeps it: %q", got)
	}
}
