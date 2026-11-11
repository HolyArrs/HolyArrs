package downloads

import (
	"bytes"
	"encoding/base64"
	"encoding/xml"
	"errors"
	"fmt"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

// xmlrpcMaxDepth bounds nested arrays and structs in an answer. rTorrent answers
// nest three deep at most (a list of rows of values).
const xmlrpcMaxDepth = 16

var errXMLRPCAnswer = errors.New("the address answered, but not like rTorrent's XML-RPC")

// xmlrpcFault is an XML-RPC fault answer.
type xmlrpcFault struct {
	Code    int64
	Message string
}

// Error implements error.
func (f *xmlrpcFault) Error() string {
	return fmt.Sprintf("rTorrent refused the call (%d): %s", f.Code, cleanDetail(f.Message))
}

// unknownHash reports a fault for an info hash rTorrent does not hold. Older
// builds say "Could not find info-hash.", newer ones "info-hash not found".
func (f *xmlrpcFault) unknownHash() bool {
	m := strings.ToLower(f.Message)
	return strings.Contains(m, "could not find info-hash") || strings.Contains(m, "info-hash not found")
}

// untrusted reports ruTorrent or rTorrent refusing a command for the
// connection, as newer builds do for calls passed through ruTorrent's plugins.
func (f *xmlrpcFault) untrusted() bool {
	return strings.Contains(strings.ToLower(f.Message), "not allowed")
}

// tooLarge reports an answer or a call over rTorrent's XML-RPC size limit.
func (f *xmlrpcFault) tooLarge() bool {
	m := strings.ToLower(f.Message)
	return strings.Contains(m, "size limit") || strings.Contains(m, "too large")
}

// xmlrpcCall encodes one method call. Arguments are strings, integers, byte
// slices (sent as base64) and slices of those.
func xmlrpcCall(method string, args ...any) ([]byte, error) {
	var b bytes.Buffer
	b.WriteString(`<?xml version="1.0"?><methodCall><methodName>`)
	_ = xml.EscapeText(&b, []byte(method))
	b.WriteString(`</methodName><params>`)
	for _, a := range args {
		b.WriteString(`<param>`)
		if err := xmlrpcValue(&b, a); err != nil {
			return nil, err
		}
		b.WriteString(`</param>`)
	}
	b.WriteString(`</params></methodCall>`)
	return b.Bytes(), nil
}

func xmlrpcValue(b *bytes.Buffer, v any) error {
	b.WriteString(`<value>`)
	switch x := v.(type) {
	case string:
		b.WriteString(`<string>`)
		_ = xml.EscapeText(b, []byte(x))
		b.WriteString(`</string>`)
	case int:
		xmlrpcInt(b, int64(x))
	case int64:
		xmlrpcInt(b, x)
	case []byte:
		b.WriteString(`<base64>`)
		b.WriteString(base64.StdEncoding.EncodeToString(x))
		b.WriteString(`</base64>`)
	case []string:
		b.WriteString(`<array><data>`)
		for _, s := range x {
			_ = xmlrpcValue(b, s)
		}
		b.WriteString(`</data></array>`)
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		slices.Sort(keys)
		b.WriteString(`<struct>`)
		for _, k := range keys {
			b.WriteString(`<member><name>`)
			_ = xml.EscapeText(b, []byte(k))
			b.WriteString(`</name>`)
			if err := xmlrpcValue(b, x[k]); err != nil {
				return err
			}
			b.WriteString(`</member>`)
		}
		b.WriteString(`</struct>`)
	case []any:
		b.WriteString(`<array><data>`)
		for _, item := range x {
			if err := xmlrpcValue(b, item); err != nil {
				return err
			}
		}
		b.WriteString(`</data></array>`)
	default:
		return fmt.Errorf("xml-rpc: cannot send a %T", v)
	}
	b.WriteString(`</value>`)
	return nil
}

func xmlrpcInt(b *bytes.Buffer, n int64) {
	tag := "i8"
	if n >= math.MinInt32 && n <= math.MaxInt32 {
		tag = "i4"
	}
	fmt.Fprintf(b, "<%s>%d</%s>", tag, n, tag)
}

// xmlrpcResponse decodes a methodResponse into its one value: string, int64,
// bool, float64, []byte, []any or map[string]any. A fault comes back as
// *xmlrpcFault.
func xmlrpcResponse(body []byte) (any, error) {
	d := xml.NewDecoder(bytes.NewReader(xmlrpcClean(body)))
	d.Strict = true
	d.Entity = map[string]string{}
	start, err := xmlrpcNext(d)
	if err != nil || start.Name.Local != "methodResponse" {
		return nil, errXMLRPCAnswer
	}
	el, err := xmlrpcNext(d)
	if err != nil {
		return nil, errXMLRPCAnswer
	}
	switch el.Name.Local {
	case "params":
		if p, err := xmlrpcNext(d); err != nil || p.Name.Local != "param" {
			return nil, errXMLRPCAnswer
		}
		if v, err := xmlrpcNext(d); err != nil || v.Name.Local != "value" {
			return nil, errXMLRPCAnswer
		}
		return xmlrpcReadValue(d, 0)
	case "fault":
		if v, err := xmlrpcNext(d); err != nil || v.Name.Local != "value" {
			return nil, errXMLRPCAnswer
		}
		v, err := xmlrpcReadValue(d, 0)
		if err != nil {
			return nil, err
		}
		m, ok := v.(map[string]any)
		if !ok {
			return nil, errXMLRPCAnswer
		}
		f := &xmlrpcFault{}
		f.Code, _ = m["faultCode"].(int64)
		f.Message, _ = m["faultString"].(string)
		return nil, f
	}
	return nil, errXMLRPCAnswer
}

// xmlrpcNext returns the next start element, skipping text, comments and the
// prolog. A directive (a DOCTYPE with entities) is refused.
func xmlrpcNext(d *xml.Decoder) (xml.StartElement, error) {
	for {
		tok, err := d.Token()
		if err != nil {
			return xml.StartElement{}, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			return t, nil
		case xml.EndElement:
			return xml.StartElement{}, errXMLRPCAnswer
		case xml.Directive:
			return xml.StartElement{}, errXMLRPCAnswer
		}
	}
}

// xmlrpcReadValue reads what follows an opened <value> up to its </value>.
func xmlrpcReadValue(d *xml.Decoder, depth int) (any, error) {
	if depth > xmlrpcMaxDepth {
		return nil, errXMLRPCAnswer
	}
	var text strings.Builder
	for {
		tok, err := d.Token()
		if err != nil {
			return nil, errXMLRPCAnswer
		}
		switch t := tok.(type) {
		case xml.CharData:
			text.Write(t)
		case xml.Directive:
			return nil, errXMLRPCAnswer
		case xml.EndElement:
			// A value with no type tag is a string.
			return text.String(), nil
		case xml.StartElement:
			v, err := xmlrpcTyped(d, t.Name.Local, depth)
			if err != nil {
				return nil, err
			}
			if err := xmlrpcClose(d); err != nil {
				return nil, err
			}
			return v, nil
		}
	}
}

// xmlrpcClose skips whitespace up to the </value> that ends a typed value.
func xmlrpcClose(d *xml.Decoder) error {
	for {
		tok, err := d.Token()
		if err != nil {
			return errXMLRPCAnswer
		}
		switch t := tok.(type) {
		case xml.EndElement:
			return nil
		case xml.CharData:
			if len(bytes.TrimSpace(t)) != 0 {
				return errXMLRPCAnswer
			}
		case xml.StartElement, xml.Directive:
			return errXMLRPCAnswer
		}
	}
}

func xmlrpcTyped(d *xml.Decoder, tag string, depth int) (any, error) {
	switch tag {
	case "array":
		return xmlrpcArray(d, depth)
	case "struct":
		return xmlrpcStruct(d, depth)
	}
	var s string
	if err := xmlrpcText(d, &s); err != nil {
		return nil, err
	}
	switch tag {
	case "string":
		return s, nil
	case "i4", "i8", "int":
		n, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
		if err != nil {
			return nil, errXMLRPCAnswer
		}
		return n, nil
	case "boolean":
		return strings.TrimSpace(s) == "1", nil
	case "double":
		f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return nil, errXMLRPCAnswer
		}
		return f, nil
	case "base64":
		b, err := base64.StdEncoding.DecodeString(strings.Join(strings.Fields(s), ""))
		if err != nil {
			return nil, errXMLRPCAnswer
		}
		return b, nil
	case "nil":
		return nil, nil
	}
	return s, nil
}

// xmlrpcText reads plain text up to the end of the current element.
func xmlrpcText(d *xml.Decoder, out *string) error {
	var b strings.Builder
	for {
		tok, err := d.Token()
		if err != nil {
			return errXMLRPCAnswer
		}
		switch t := tok.(type) {
		case xml.CharData:
			b.Write(t)
		case xml.EndElement:
			*out = b.String()
			return nil
		case xml.StartElement, xml.Directive:
			return errXMLRPCAnswer
		}
	}
}

func xmlrpcArray(d *xml.Decoder, depth int) (any, error) {
	if s, err := xmlrpcNext(d); err != nil || s.Name.Local != "data" {
		return nil, errXMLRPCAnswer
	}
	out := []any{}
	for {
		tok, err := d.Token()
		if err != nil {
			return nil, errXMLRPCAnswer
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local != "value" {
				return nil, errXMLRPCAnswer
			}
			v, err := xmlrpcReadValue(d, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, v)
		case xml.EndElement:
			// </data>, then </array>.
			if err := xmlrpcClose(d); err != nil {
				return nil, err
			}
			return out, nil
		case xml.Directive:
			return nil, errXMLRPCAnswer
		}
	}
}

func xmlrpcStruct(d *xml.Decoder, depth int) (any, error) {
	out := map[string]any{}
	for {
		tok, err := d.Token()
		if err != nil {
			return nil, errXMLRPCAnswer
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Local != "member" {
				return nil, errXMLRPCAnswer
			}
			n, err := xmlrpcNext(d)
			if err != nil || n.Name.Local != "name" {
				return nil, errXMLRPCAnswer
			}
			var name string
			if err := xmlrpcText(d, &name); err != nil {
				return nil, err
			}
			if v, err := xmlrpcNext(d); err != nil || v.Name.Local != "value" {
				return nil, errXMLRPCAnswer
			}
			v, err := xmlrpcReadValue(d, depth+1)
			if err != nil {
				return nil, err
			}
			out[name] = v
			if err := xmlrpcClose(d); err != nil {
				return nil, err
			}
		case xml.EndElement:
			return out, nil
		case xml.Directive:
			return nil, errXMLRPCAnswer
		}
	}
}

// badCharRef matches character references to control characters, which XML
// forbids and the decoder refuses.
var badCharRef = regexp.MustCompile(`&#(x0*[0-8bBcCeEfF]|x0*1[0-9a-fA-F]|0*([0-8]|1[124-9]|2[0-9]|3[01]));`)

// xmlrpcClean makes an answer decodable when a torrent name or message holds
// bytes XML does not allow: invalid UTF-8 and control characters become U+FFFD,
// so one odd name never hides every other torrent.
func xmlrpcClean(b []byte) []byte {
	bad := func(r rune) bool { return r < 0x20 && r != '\t' && r != '\n' && r != '\r' }
	b = bytes.ToValidUTF8(b, []byte("\uFFFD"))
	if bytes.IndexFunc(b, bad) >= 0 {
		b = bytes.Map(func(r rune) rune {
			if bad(r) {
				return '\uFFFD'
			}
			return r
		}, b)
	}
	if bytes.Contains(b, []byte("&#")) {
		b = badCharRef.ReplaceAll(b, []byte("\uFFFD"))
	}
	return b
}

// cleanDetail folds whitespace and cuts a client message to 160 characters.
func cleanDetail(s string) string {
	s = strings.Join(strings.Fields(s), " ")
	if r := []rune(s); len(r) > 160 {
		s = string(r[:160])
	}
	return s
}
