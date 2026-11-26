package subtext

import "testing"

func TestReviewDecodeEdges(t *testing.T) {
	if s, enc, err := Decode([]byte("Zażółć gęślą jaźń"), "pl"); err != nil || s != "Zażółć gęślą jaźń" || enc != "utf-8" {
		t.Errorf("valid UTF-8 changed: %q %s %v", s, enc, err)
	}
	if s, _, err := Decode(nil, "pl"); err != nil || s != "" {
		t.Errorf("empty: %q %v", s, err)
	}
	if s, _, err := Decode([]byte("\xef\xbb\xbfHi"), "zh"); err != nil || s != "Hi" {
		t.Errorf("BOM: %q %v", s, err)
	}
	// Unknown language with a high byte must not fail.
	if _, _, err := Decode([]byte("caf\xe9"), "xx"); err != nil {
		t.Errorf("unknown lang: %v", err)
	}
	// Plain ASCII in a Chinese wish stays as it is.
	if s, _, err := Decode([]byte("1\n00:00:01,000 --> 00:00:02,000\nOK\n"), "zh"); err != nil || s != "1\n00:00:01,000 --> 00:00:02,000\nOK\n" {
		t.Errorf("ascii: %q %v", s, err)
	}
}
