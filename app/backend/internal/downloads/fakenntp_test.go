package downloads

import (
	"bufio"
	"bytes"
	"fmt"
	"hash/crc32"
	"net"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeNNTP struct {
	ln       net.Listener
	mu       sync.Mutex
	articles map[string][]byte
	asked    int
	delay    time.Duration
}

func TestYencPartRoundTrip(t *testing.T) {
	data := make([]byte, 3000)
	for i := range data {
		data[i] = byte(i * 7)
	}
	enc := yencPart("x.bin", data, 1, 1, 1, len(data))
	var out []byte
	for _, line := range strings.Split(string(enc), "\r\n") {
		if strings.HasPrefix(line, "=y") || line == "" {
			continue
		}
		for i := 0; i < len(line); i++ {
			c := line[i]
			if c == '=' {
				i++
				c = line[i] - 64
			}
			out = append(out, c-42)
		}
	}
	if !bytes.Equal(out, data) {
		t.Fatalf("round trip lost bytes: %d vs %d", len(out), len(data))
	}
}

func yencPart(name string, whole []byte, part, parts, begin, end int) []byte {
	data := whole[begin-1 : end]
	var b bytes.Buffer
	fmt.Fprintf(&b, "=ybegin part=%d total=%d line=128 size=%d name=%s\r\n", part, parts, len(whole), name)
	fmt.Fprintf(&b, "=ypart begin=%d end=%d\r\n", begin, end)
	col := 0
	for _, c := range data {
		o := byte(int(c) + 42)
		esc := o == 0 || o == '\n' || o == '\r' || o == '=' || (col == 0 && (o == '.' || o == '\t' || o == ' ')) || (col == 127 && (o == '\t' || o == ' '))
		if esc {
			b.WriteByte('=')
			o += 64
			col++
		}
		b.WriteByte(o)
		col++
		if col >= 128 {
			b.WriteString("\r\n")
			col = 0
		}
	}
	if col > 0 {
		b.WriteString("\r\n")
	}
	fmt.Fprintf(&b, "=yend size=%d part=%d pcrc32=%08x\r\n", len(data), part, crc32.ChecksumIEEE(data))
	return b.Bytes()
}

func (f *fakeNNTP) makeNZB(name string, data []byte, partSize int) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.articles == nil {
		f.articles = map[string][]byte{}
	}
	parts := (len(data) + partSize - 1) / partSize
	var segs strings.Builder
	for i := range parts {
		begin, end := i*partSize+1, min((i+1)*partSize, len(data))
		id := fmt.Sprintf("part%d.%s@holyarrs.test", i+1, strings.ReplaceAll(name, " ", "_"))
		body := yencPart(name, data, i+1, parts, begin, end)
		f.articles[id] = body
		fmt.Fprintf(&segs, `<segment bytes="%d" number="%d">%s</segment>`, len(body), i+1, id)
	}
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE nzb PUBLIC "-//newzBin//DTD NZB 1.1//EN" "http://www.newzbin.com/DTD/nzb/nzb-1.1.dtd">
<nzb xmlns="http://www.newzbin.com/DTD/2003/nzb">
<file poster="lab@holyarrs.test" date="1700000000" subject="` + fmt.Sprintf("[1/1] - &quot;%s&quot; yEnc (1/%d)", name, parts) + `">
<groups><group>alt.binaries.holyarrs.test</group></groups>
<segments>` + segs.String() + `</segments>
</file>
</nzb>
`
}

func startFakeNNTP(t *testing.T, addr string) *fakeNNTP {
	t.Helper()
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		t.Fatal(err)
	}
	f := &fakeNNTP{ln: ln, articles: map[string][]byte{}}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go f.serve(c)
		}
	}()
	return f
}

func (f *fakeNNTP) serve(c net.Conn) {
	defer c.Close()
	r := bufio.NewReader(c)
	w := bufio.NewWriter(c)
	say := func(s string) { _, _ = w.WriteString(s + "\r\n"); _ = w.Flush() }
	say("200 holyarrs lab news server ready")
	for {
		line, err := r.ReadString('\n')
		if err != nil {
			return
		}
		fields := strings.Fields(strings.TrimSpace(line))
		if len(fields) == 0 {
			continue
		}
		cmd := strings.ToUpper(fields[0])
		arg := ""
		if len(fields) > 1 {
			arg = fields[1]
		}
		switch cmd {
		case "CAPABILITIES":
			say("101 list follows\r\nVERSION 2\r\nREADER\r\n.")
		case "MODE":
			say("200 reader mode")
		case "AUTHINFO":
			if len(fields) > 1 && strings.EqualFold(fields[1], "USER") {
				say("381 password please")
			} else {
				say("281 welcome")
			}
		case "GROUP":
			say("211 1 1 1 " + arg)
		case "DATE":
			say("111 20261013000000")
		case "QUIT":
			say("205 bye")
			return
		case "BODY", "ARTICLE", "HEAD", "STAT":
			id := strings.Trim(arg, "<>")
			f.mu.Lock()
			body, ok := f.articles[id]
			f.asked++
			delay := f.delay
			f.mu.Unlock()
			time.Sleep(delay)
			if !ok {
				say("430 no such article")
				continue
			}
			switch cmd {
			case "STAT":
				say("223 0 <" + id + ">")
				continue
			case "HEAD":
				say("221 0 <" + id + ">\r\nMessage-ID: <" + id + ">\r\nSubject: lab\r\n.")
				continue
			case "ARTICLE":
				_, _ = w.WriteString("220 0 <" + id + ">\r\nMessage-ID: <" + id + ">\r\nSubject: lab\r\n\r\n")
			default:
				_, _ = w.WriteString("222 0 <" + id + ">\r\n")
			}
			for _, l := range strings.SplitAfter(string(body), "\r\n") {
				if l == "" {
					continue
				}
				if strings.HasPrefix(l, ".") {
					_, _ = w.WriteString(".")
				}
				_, _ = w.WriteString(l)
			}
			say(".")
		default:
			say("500 unknown command")
		}
	}
}
