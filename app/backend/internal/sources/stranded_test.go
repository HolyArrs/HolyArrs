package sources

import (
	"context"
	"strings"
	"testing"
)

// A title no Source would be asked for is counted for Settings: every Source
// has tags and the title shares none. A tag on the title, on its Pick rules,
// or a Source with no tags takes it off the count.
func TestStranded(t *testing.T) {
	lf := &liveFeed{}
	lf.set(duneFeed)
	f := seed(t, lf.url(t))
	ctx := context.Background()
	count := func(want int) string {
		t.Helper()
		n, text, err := Stranded(ctx, f.s)
		if err != nil || n != want {
			t.Fatalf("stranded %d (%q, %v), want %d", n, text, err, want)
		}
		return text
	}

	count(0) // an untagged Source serves every title
	exec(t, f, `UPDATE sources SET tags = '["toloka"]'`)
	if text := count(1); !strings.HasPrefix(text, "1 title has no Source") {
		t.Fatalf("text %q", text)
	}
	exec(t, f, `UPDATE titles SET tags = '["toloka"]'`)
	count(0)
	exec(t, f, `UPDATE titles SET tags = '[]'`)
	exec(t, f, `UPDATE pick_rules SET body = json_set(body, '$.tags', json('["toloka"]'))`)
	count(0)

	// Pick rules with a tag no Source has leave every Source out.
	exec(t, f, `UPDATE pick_rules SET body = json_set(body, '$.tags', json('["hd"]'))`)
	count(1)

	// A title not kept up to date is never searched on its own, and with no
	// Source that searches on its own there is nothing to say here.
	exec(t, f, `UPDATE titles SET keep_up_to_date = 0`)
	count(0)
	exec(t, f, `UPDATE titles SET keep_up_to_date = 1`)
	exec(t, f, `UPDATE sources SET state = 'disabled'`)
	count(0)
}
