package herovideo

import (
	"fmt"
	"math"
	"strings"
	"testing"
)

// realSpec has the shape of YouTube's storyboard spec for a 634 s video:
// one 10x10 sheet of 48x27 over the whole video, then 80x45, 160x90 and
// 320x180 tiles 2 s apart.
const realSpec = "https://i.ytimg.com/sb/aqz-KE-bpKQ/storyboard3_L$L/$N.jpg?sqp=-oaymwENSDfyq4qpAwVwAcABBqLzl_8DBgjL_PDBBg==" +
	"|48#27#100#10#10#0#default#rs$AOn4CLDl1" +
	"|80#45#318#10#10#2000#M$M#rs$AOn4CLB2" +
	"|160#90#318#5#5#2000#M$M#rs$AOn4CLCx" +
	"|320#180#318#3#3#2000#M$M#rs$M0n4CLD7"

func TestStoryboardsReadsTheSpec(t *testing.T) {
	fs := storyboards(realSpec, 634)
	if len(fs) != 4 {
		t.Fatalf("%d sizes", len(fs))
	}
	q := "?sqp=-oaymwENSDfyq4qpAwVwAcABBqLzl_8DBgjL_PDBBg=="
	// The smallest: one sheet named "default" covering the whole video.
	s0 := fs[0]
	if s0.Width != 48 || s0.Height != 27 || s0.Columns != 10 || s0.Rows != 10 || len(s0.Fragments) != 1 ||
		s0.Fragments[0].URL != "https://i.ytimg.com/sb/aqz-KE-bpKQ/storyboard3_L0/default.jpg"+q+"&sigh=rs$AOn4CLDl1" ||
		math.Abs(s0.step()-6.34) > 1e-9 || math.Abs(s0.Fragments[0].Duration-634) > 1e-9 {
		t.Fatalf("size 0 %+v step %v", s0, s0.step())
	}
	// 2 s apart: 318 tiles in four sheets of 100, the last one shorter.
	s1 := fs[1]
	var spans []string
	for _, f := range s1.Fragments {
		spans = append(spans, fmt.Sprint(f.Duration))
	}
	if s1.step() != 2 || len(s1.Fragments) != 4 || strings.Join(spans, " ") != "200 200 200 34" ||
		s1.Fragments[0].URL != "https://i.ytimg.com/sb/aqz-KE-bpKQ/storyboard3_L1/M0.jpg"+q+"&sigh=rs$AOn4CLB2" ||
		!strings.Contains(s1.Fragments[3].URL, "/storyboard3_L1/M3.jpg?") {
		t.Fatalf("size 1 %+v spans %v", s1, spans)
	}
	// A signature that holds "$M" is kept as it is.
	s3 := fs[3]
	if len(s3.Fragments) != 36 || !strings.HasSuffix(s3.Fragments[35].URL, "/storyboard3_L3/M35.jpg"+q+"&sigh=rs$M0n4CLD7") {
		t.Fatalf("size 3 %d sheets, last %q", len(s3.Fragments), s3.Fragments[len(s3.Fragments)-1].URL)
	}
	// The start is judged on the 2 s tiles within six sheets; the
	// storyboard check reads the smallest.
	if f, ok := startFormat(fs); !ok || f.Width != 80 {
		t.Fatalf("start format %+v %v", f, ok)
	}
}

// Without the length, sizes with their own step still have one; the one
// sheet over the whole video does not (so it never judges a start).
func TestStoryboardsWithoutLength(t *testing.T) {
	fs := storyboards(realSpec, 0)
	if len(fs) != 4 || fs[0].step() != 0 || fs[1].step() != 2 || fs[1].Fragments[0].Duration != 200 {
		t.Fatalf("%+v", fs[:2])
	}
}

func TestStoryboardsLeavesOutBrokenSizes(t *testing.T) {
	for _, spec := range []string{"", "https://i.ytimg.com/sb/x/$L/$N.jpg", "no address|bad", "x|48#27#100#10#10#0#default"} {
		if fs := storyboards(spec, 100); len(fs) != 0 {
			t.Fatalf("%q: %+v", spec, fs)
		}
	}
	// A broken size is left out; the others keep their own numbers ($L).
	fs := storyboards("https://i.ytimg.com/sb/x/L$L/$N.jpg?a=1|0#27#100#10#10#0#default#s|48#27#x#10#10#0#default#s|80#45#10#5#5#-1#M$M#s|80#45#10#5#5#1000#M$M#s", 10)
	if len(fs) != 1 || fs[0].Fragments[0].URL != "https://i.ytimg.com/sb/x/L3/M0.jpg?a=1&sigh=s" {
		t.Fatalf("%+v", fs)
	}
	// A size claiming more sheets than any video has is left out.
	if fs := storyboards("https://i.ytimg.com/sb/x/$N.jpg|48#27#999999999#1#1#1000#M$M#s", 0); len(fs) != 0 {
		t.Fatal("huge size kept")
	}
	// An address without a host is on YouTube's picture host; no signature,
	// no sigh.
	fs = storyboards("/sb/x/$L/$N.jpg|48#27#25#5#5#0#default#", 50)
	if len(fs) != 1 || fs[0].Fragments[0].URL != "https://i.ytimg.com/sb/x/0/default.jpg" {
		t.Fatalf("%+v", fs)
	}
}
