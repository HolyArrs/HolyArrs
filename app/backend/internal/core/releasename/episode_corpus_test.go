package releasename

import (
	"bufio"
	"os"
	"strconv"
	"strings"
	"testing"
)

func fmtInts(v []int) string {
	var b strings.Builder
	for i, n := range v {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(strconv.Itoa(n))
	}
	return b.String()
}

// TestEpisodeCorpus reads every name in testdata/episode_corpus.tsv and
// checks what is read against the row. A row with a leading ! is a known
// miss: it is logged, and the test says when it starts reading right.
func TestEpisodeCorpus(t *testing.T) {
	f, err := os.Open("testdata/episode_corpus.tsv")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	rows, pass := 0, 0
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		known := strings.HasPrefix(line, "!")
		p := strings.Split(strings.TrimPrefix(line, "!"), "\t")
		if len(p) != 4 {
			t.Fatalf("bad row %q", line)
		}
		rows++
		r := Parse(p[2], Hints{Show: p[1] == "y"})
		got := [6]string{r.Kind.Value, "", "", "", "", r.Title.Value}
		if r.Show != nil {
			got[1], got[2], got[3], got[4] = fmtInts(r.Show.Seasons), fmtInts(r.Show.Episodes), fmtInts(r.Show.Absolute), r.Show.AirDate
		}
		ok := true
		if p[3] != "" {
			for i, w := range strings.Split(p[3], "|") {
				if (i == 0 || w != "") && got[i] != w {
					ok = false
				}
			}
		}
		switch {
		case ok:
			pass++
			if known {
				t.Errorf("[%s] %q now reads right: drop the ! from its row", p[0], p[2])
			}
		case known:
			t.Logf("known miss [%s] %q want %q got %q", p[0], p[2], p[3], got)
		default:
			t.Errorf("[%s] %q want %q got %q", p[0], p[2], p[3], got)
		}
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	t.Logf("%d names, %d read right", rows, pass)
}

// An anime name keeps its leading [group] over a last "[Batch]" bracket.
func TestAnimeLeadGroupWins(t *testing.T) {
	for _, n := range []string{"[Group] Ember Blade - 01-12 [BD 1080p] [Batch]", "[Group] Ember Blade - 05 [1080p] [Multi-Sub].mkv", "[Group].Ember.Blade.-.05.[1080p].mkv"} {
		if g := Parse(n, Hints{}).Group.Value; g != "Group" {
			t.Errorf("%s: group %q", n, g)
		}
	}
}
