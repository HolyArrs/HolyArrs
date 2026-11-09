package music

import (
	"strings"
)

// musicShape is an answer's rows as "id: key key ..." lines (hero first).
func musicShape(m musicRowsAnswer) string {
	var b strings.Builder
	if m.Hero != nil {
		b.WriteString("hero: " + m.Hero.Key + "\n")
	}
	for _, r := range m.Rows {
		b.WriteString(r.ID + " [" + r.Shape + "] " + r.Title + ":")
		for _, c := range r.Items {
			b.WriteString(" " + c.Key)
		}
		b.WriteString("\n")
	}
	return b.String()
}
