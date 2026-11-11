// Package translate turns the lines of a subtitle into another language with
// a translator the owner runs or pays for: any server that speaks the OpenAI
// chat API (cloud services, Ollama, LM Studio) or a Lingarr server. Lines go
// out in numbered chunks. Every answer is checked for missing, extra and
// repeated numbers; missing lines are asked again in smaller chunks, and a
// line that never comes back keeps its source text, so the timing of every
// cue stays exactly where it was.
package translate

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Line is the text of one cue with its number in the file.
type Line struct {
	N    int
	Text string
}

// Batch is what one call to a translator carries.
type Batch struct {
	From, To         string // language codes, such as en and pt-br
	FromName, ToName string // English language names, such as Portuguese (Brazil)
	Title            string
	Kind             string // movie or episode
	MediaID          int64
	Lines            []Line
}

// Client is one translator. Translate returns the lines it got back, which
// may be fewer, more or in another order; Run checks them.
type Client interface {
	Translate(ctx context.Context, b Batch) ([]Line, error)
}

// Error kinds. Refused stops a run at once; Busy and Down are tried again
// after a wait; Cut and Unreadable are asked again in smaller chunks.
const (
	KindRefused    = "refused"    // the key, model or address was refused
	KindBusy       = "busy"       // too many calls, or the server failed
	KindDown       = "down"       // the server did not answer
	KindCut        = "cut"        // the answer stopped early
	KindUnreadable = "unreadable" // the answer could not be read
)

// Error is a failed call. It never holds the address or the key, so it is
// safe to log and show.
type Error struct {
	Kind       string
	Msg        string
	RetryAfter time.Duration
}

func (e *Error) Error() string { return "translator " + e.Kind + ": " + e.Msg }

// KindOf returns the kind of a translator error, or Down for any other error.
func KindOf(err error) string {
	var e *Error
	if errors.As(err, &e) {
		return e.Kind
	}
	return KindDown
}

// Limits of one run.
const (
	DefaultChunk = 40    // lines in the first calls
	MaxChunk     = 200   // lines a chunk may ever hold
	MaxLines     = 20000 // lines in one file
	maxLineTries = 3     // calls one line may take part in before it keeps its source text
	callTries    = 3     // tries of one call when the server is busy or down
	maxFruitless = 8     // calls in a row that bring back no line before a run gives up
)

// Job is a whole file to translate: the batch details with every line.
type Job struct {
	Batch
	Chunk int // lines per call at first; DefaultChunk when 0
}

// Result is what a run made. Lines holds one text per input line: the
// translation, or the source text when none came back.
type Result struct {
	Lines      []string
	Done       []bool
	Translated int // lines that came back translated
	Total      int // lines that needed a translation (blank lines need none)
	Calls      int
	Extra      int // numbers a translator sent that were never asked for
}

// Partial reports whether some lines kept their source text.
func (r Result) Partial() bool { return r.Translated < r.Total }

// wait sleeps for d or until ctx ends. Tests swap it to skip the waits.
var wait = func(ctx context.Context, d time.Duration) error {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Run translates every line of j with c. It returns what it made even when
// it stops early with an error, so the caller can keep a partial result.
func Run(ctx context.Context, c Client, j Job) (Result, error) {
	n := len(j.Lines)
	res := Result{Lines: make([]string, n), Done: make([]bool, n)}
	if n > MaxLines {
		return res, &Error{Kind: KindUnreadable, Msg: fmt.Sprintf("the subtitle has over %d lines", MaxLines)}
	}
	start := j.Chunk
	if start <= 0 {
		start = DefaultChunk
	}
	start = min(start, MaxChunk)
	var pending []int
	for i, l := range j.Lines {
		res.Lines[i] = l.Text
		if strings.TrimSpace(l.Text) == "" {
			res.Done[i] = true // nothing to translate
			continue
		}
		pending = append(pending, i)
	}
	res.Total = len(pending)
	size, good, cursor, fruitless := start, 0, 0, 0
	tries := make([]int, n)
	var again [][]int // chunks to ask again, before new ones
	for len(again) > 0 || cursor < len(pending) {
		if err := ctx.Err(); err != nil {
			return res, err
		}
		var chunk []int
		if len(again) > 0 {
			chunk, again = again[0], again[1:]
		} else {
			end := min(cursor+size, len(pending))
			chunk, cursor = pending[cursor:end], end
		}
		b := j.Batch
		b.Lines = make([]Line, len(chunk))
		for k, i := range chunk {
			b.Lines[k] = j.Lines[i]
		}
		got, err := call(ctx, c, b, &res.Calls)
		if err != nil {
			switch KindOf(err) {
			case KindCut, KindUnreadable:
				// A translator that keeps failing this way is given up on, so a
				// broken or too slow model never spends on every line.
				if fruitless++; fruitless >= maxFruitless {
					return res, err
				}
				// A smaller chunk usually fits and reads; one line that keeps
				// failing keeps its source text.
				size, good = max(1, len(chunk)/2), 0
				again = append(retryChunks(chunk, tries, size), again...)
				continue
			}
			return res, err
		}
		byN := map[int]int{}
		for k, i := range chunk {
			byN[j.Lines[i].N] = k
		}
		filled := make([]bool, len(chunk))
		for _, l := range got {
			k, ok := byN[l.N]
			if !ok {
				res.Extra++
				continue
			}
			text := cleanReply(l.Text)
			if filled[k] || text == "" {
				continue // a repeated number keeps the first answer
			}
			filled[k] = true
			i := chunk[k]
			res.Lines[i], res.Done[i] = text, true
			res.Translated++
		}
		var missing []int
		for k, ok := range filled {
			if !ok {
				missing = append(missing, chunk[k])
			}
		}
		if len(missing) < len(chunk) {
			fruitless = 0
		} else if fruitless++; fruitless >= maxFruitless {
			return res, &Error{Kind: KindUnreadable, Msg: "the translator keeps sending back no usable lines"}
		}
		if len(missing) == 0 {
			if good++; good >= 3 && size < start {
				size, good = min(start, size*2), 0
			}
			continue
		}
		good = 0
		again = append(retryChunks(missing, tries, max(1, len(chunk)/2)), again...)
	}
	return res, nil
}

// retryChunks counts one more try for each line and splits the lines that may
// still be tried into chunks of at most size.
func retryChunks(lines []int, tries []int, size int) [][]int {
	var keep []int
	for _, i := range lines {
		if tries[i]++; tries[i] < maxLineTries {
			keep = append(keep, i)
		}
	}
	var out [][]int
	for len(keep) > 0 {
		k := min(size, len(keep))
		out = append(out, keep[:k:k])
		keep = keep[k:]
	}
	return out
}

// call asks c once, trying again after a wait while the server is busy or
// down. A refused call is never tried again.
func call(ctx context.Context, c Client, b Batch, calls *int) ([]Line, error) {
	var err error
	for try := 0; try < callTries; try++ {
		if try > 0 {
			d := time.Duration(1<<try) * time.Second
			var te *Error
			if errors.As(err, &te) && te.RetryAfter > 0 {
				d = min(te.RetryAfter, 30*time.Second)
			}
			if werr := wait(ctx, d); werr != nil {
				return nil, werr
			}
		}
		*calls++
		var got []Line
		got, err = c.Translate(ctx, b)
		if err == nil {
			return got, nil
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if k := KindOf(err); k != KindBusy && k != KindDown {
			return nil, err
		}
	}
	return nil, err
}

// cleanReply turns HTML line breaks into real ones and trims the text.
func cleanReply(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	for _, br := range []string{"<br />", "<br/>", "<br>", "<BR>", "<BR/>", "<BR />"} {
		s = strings.ReplaceAll(s, br, "\n")
	}
	return strings.TrimSpace(s)
}
