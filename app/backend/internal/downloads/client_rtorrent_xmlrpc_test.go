package downloads

import (
	"errors"
	"strings"
	"testing"
)

func TestXMLRPCEncodeCall(t *testing.T) {
	b, err := xmlrpcCall("load.raw_start", "", []byte("d4:infoe"), "d.custom1.set=movies", int64(7), []any{"a", int64(1)})
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	for _, want := range []string{
		"<methodName>load.raw_start</methodName>",
		"<param><value><string></string></value></param>",
		"<base64>ZDQ6aW5mb2U=</base64>",
		"<string>d.custom1.set=movies</string>",
		"<i4>7</i4>",
		"<array><data><value><string>a</string></value><value><i4>1</i4></value></data></array>",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("call lacks %q:\n%s", want, s)
		}
	}
	if b, _ := xmlrpcCall("x", "<&>\"'"); !strings.Contains(string(b), "&lt;&amp;&gt;") {
		t.Errorf("text not escaped: %s", b)
	}
	if _, err := xmlrpcCall("x", struct{}{}); err == nil {
		t.Error("an unknown argument type was encoded")
	}
}

func TestXMLRPCDecodeValues(t *testing.T) {
	body := `<?xml version="1.0" encoding="UTF-8"?>
<methodResponse><params><param><value><array><data>
 <value><array><data>
  <value><string>ABCDEF</string></value>
  <value>bare text</value>
  <value><i8>10737418240</i8></value>
  <value><i4>-3</i4></value>
  <value><int>5</int></value>
  <value><string/></value>
  <value></value>
  <value><boolean>1</boolean></value>
  <value><double>1.5</double></value>
  <value><base64>aGk=</base64></value>
  <value><struct><member><name>k</name><value><i4>2</i4></value></member></struct></value>
 </data></array></value>
</data></array></value></param></params></methodResponse>`
	v, err := xmlrpcResponse([]byte(body))
	if err != nil {
		t.Fatal(err)
	}
	rows, ok := v.([]any)
	if !ok || len(rows) != 1 {
		t.Fatalf("rows: %#v", v)
	}
	row := rows[0].([]any)
	want := []any{"ABCDEF", "bare text", int64(10737418240), int64(-3), int64(5), "", "", true, 1.5, []byte("hi")}
	for i, w := range want {
		got := row[i]
		if gb, ok := got.([]byte); ok {
			if string(gb) != string(w.([]byte)) {
				t.Errorf("value %d: %q, want %q", i, gb, w)
			}
			continue
		}
		if got != w {
			t.Errorf("value %d: %#v, want %#v", i, got, w)
		}
	}
	if m, ok := row[10].(map[string]any); !ok || m["k"] != int64(2) {
		t.Errorf("struct: %#v", row[10])
	}
}

func TestXMLRPCDecodeFault(t *testing.T) {
	body := `<methodResponse><fault><value><struct>
<member><name>faultCode</name><value><i4>-501</i4></value></member>
<member><name>faultString</name><value><string>Could not find info-hash.</string></value></member>
</struct></value></fault></methodResponse>`
	_, err := xmlrpcResponse([]byte(body))
	var f *xmlrpcFault
	if !errors.As(err, &f) || f.Code != -501 || f.Message != "Could not find info-hash." {
		t.Fatalf("fault: %#v %v", f, err)
	}
	if !f.unknownHash() {
		t.Error("an unknown hash fault was not recognised")
	}
	newer := `<methodResponse><fault><value><struct><member><name>faultCode</name><value><i8>-500</i8></value></member><member><name>faultString</name><value><string>invalid parameters: info-hash not found</string></value></member></struct></value></fault></methodResponse>`
	if _, err := xmlrpcResponse([]byte(newer)); !errors.As(err, &f) || !f.unknownHash() || f.Code != -500 {
		t.Errorf("rTorrent 0.16 unknown hash fault: %v", err)
	}
	if _, err := xmlrpcResponse([]byte(`<methodResponse><params><param><value><array><data/></array></value></param></params></methodResponse>`)); err != nil {
		t.Errorf("empty list: %v", err)
	}
}

func TestXMLRPCDecodeRefusesJunk(t *testing.T) {
	for name, body := range map[string]string{
		"html":     `<html><body>login</body></html>`,
		"empty":    ``,
		"no value": `<methodResponse><params><param></param></params></methodResponse>`,
		"bad int":  `<methodResponse><params><param><value><i4>x</i4></value></param></params></methodResponse>`,
		"doctype":  `<!DOCTYPE x [<!ENTITY a "b">]><methodResponse><params><param><value>&a;</value></param></params></methodResponse>`,
		"deep":     `<methodResponse><params><param><value>` + strings.Repeat("<array><data><value>", 80) + `x` + strings.Repeat("</value></data></array>", 80) + `</value></param></params></methodResponse>`,
	} {
		if _, err := xmlrpcResponse([]byte(body)); err == nil {
			t.Errorf("%s: decoded", name)
		}
	}
}
