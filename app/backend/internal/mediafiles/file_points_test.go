package mediafiles

import "testing"

func TestScoreFile(t *testing.T) {
	r := GoodEveryday()
	need := 100
	r.GoodEnoughPoints = &need
	r.Traits = []TraitPoints{{Trait: "dv", Points: 40}, {Trait: "web_dl", Points: 25}, {Trait: "remux", Points: 500}}
	p := ScoreFile("Show.S01E01.2160p.WEB-DL.DDP5.1.DV.HDR.H.265-GRP", 0, nil, r)
	if !p.StepReached || p.Step == "" || p.GoodEnoughAt != r.GoodEnoughAt {
		t.Fatalf("step: %+v", p)
	}
	if p.GoodEnoughPoints == nil || *p.GoodEnoughPoints != 100 {
		t.Fatalf("good enough points: %+v", p.GoodEnoughPoints)
	}
	sum := 0
	for _, ts := range p.Traits {
		if ts.Trait == "remux" {
			t.Errorf("remux does not fit a WEB-DL: %+v", p.Traits)
		}
		sum += ts.Points
	}
	if p.Points != 65 || sum != p.Points {
		t.Errorf("traits add to %d, total %d", sum, p.Points)
	}

	low := ScoreFile("Show.S01E01.720p.WEB-DL.H.264-GRP", 0, nil, r)
	if low.StepReached {
		t.Errorf("720p is below good enough: %+v", low)
	}
	none := ScoreFile("no quality at all", 0, nil, r)
	if none.StepReached || none.Traits == nil {
		t.Errorf("unknown quality: %+v", none)
	}
}
