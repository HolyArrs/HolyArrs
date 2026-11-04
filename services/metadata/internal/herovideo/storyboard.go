package herovideo

import (
	"math"
	"strconv"
	"strings"
)

// sbFormat is one storyboard size: sheets of Columns x Rows tiles of
// Width x Height, FPS tiles a second (0: unknown).
type sbFormat struct {
	Width, Height, Columns, Rows int
	FPS                          float64
	Fragments                    []sbSheet
}

// sbSheet is one sheet's address and the seconds it covers (0: unknown).
type sbSheet struct {
	URL      string
	Duration float64
}

// maxBoardSheets leaves out a size listing more sheets than any real video
// has (a broken spec).
const maxBoardSheets = 10000

// storyboards reads YouTube's storyboard spec into its sizes, smallest
// first as YouTube lists them. The spec is the sheet address, then one
// "|" part per size: width#height#tiles#columns#rows#ms between
// tiles#name#signature. In the address $L is the size's number, $N its
// name and $M the sheet's number; the signature goes on as sigh. dur is
// the video's length in seconds (0: unknown). A malformed size is left out.
func storyboards(spec string, dur float64) []sbFormat {
	parts := strings.Split(strings.TrimSpace(spec), "|")
	if len(parts) < 2 {
		return nil
	}
	// An address without a host is on YouTube's picture host.
	addr := parts[0]
	switch {
	case strings.HasPrefix(addr, "//"):
		addr = "https:" + addr
	case strings.HasPrefix(addr, "/"):
		addr = "https://i.ytimg.com" + addr
	}
	var out []sbFormat
	for level, p := range parts[1:] {
		f := strings.Split(p, "#")
		if len(f) != 8 {
			continue
		}
		var n [5]int
		ok := true
		for i := range n {
			v, err := strconv.Atoi(f[i])
			n[i], ok = v, ok && err == nil && v > 0
		}
		ms, err := strconv.Atoi(f[5])
		if !ok || err != nil || ms < 0 {
			continue
		}
		w, h, tiles, cols, rows := n[0], n[1], n[2], n[3], n[4]
		sheets := (tiles + cols*rows - 1) / (cols * rows)
		if sheets > maxBoardSheets {
			continue
		}
		fps := 0.0
		switch {
		case ms > 0:
			fps = 1000 / float64(ms)
		case dur > 0:
			fps = float64(tiles) / dur
		}
		span := 0.0
		if fps > 0 {
			span = float64(cols*rows) / fps
		}
		u := strings.NewReplacer("$L", strconv.Itoa(level), "$N", f[6]).Replace(addr)
		sb := sbFormat{Width: w, Height: h, Columns: cols, Rows: rows, FPS: fps}
		for m := range sheets {
			// $M before the signature, which may hold "$M" itself.
			sheet := strings.ReplaceAll(u, "$M", strconv.Itoa(m))
			if f[7] != "" {
				sep := "?"
				if strings.Contains(sheet, "?") {
					sep = "&"
				}
				sheet += sep + "sigh=" + f[7]
			}
			d := span
			if dur > 0 && span > 0 {
				d = math.Max(0, math.Min(span, dur-float64(m)*span))
			}
			sb.Fragments = append(sb.Fragments, sbSheet{URL: sheet, Duration: d})
		}
		out = append(out, sb)
	}
	return out
}
