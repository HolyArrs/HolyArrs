package mbdump

import "strings"

// Field is one column of a PostgreSQL COPY text row.
type Field struct {
	S    string
	Null bool
}

// Fields splits one COPY text line (tab separated, \N for null) and undoes
// its backslash escapes.
func Fields(line string) []Field {
	parts := strings.Split(line, "\t")
	out := make([]Field, len(parts))
	for i, p := range parts {
		if p == `\N` {
			out[i] = Field{Null: true}
			continue
		}
		if strings.IndexByte(p, '\\') < 0 {
			out[i] = Field{S: p}
			continue
		}
		var b strings.Builder
		b.Grow(len(p))
		for j := 0; j < len(p); j++ {
			c := p[j]
			if c != '\\' || j+1 == len(p) {
				b.WriteByte(c)
				continue
			}
			j++
			switch p[j] {
			case 't':
				b.WriteByte('\t')
			case 'n':
				b.WriteByte('\n')
			case 'r':
				b.WriteByte('\r')
			case 'b':
				b.WriteByte('\b')
			case 'f':
				b.WriteByte('\f')
			case 'v':
				b.WriteByte('\v')
			default:
				b.WriteByte(p[j])
			}
		}
		out[i] = Field{S: b.String()}
	}
	return out
}
