package main

import (
	"io"
	"testing"
)

func TestParseFlagsDefaults(t *testing.T) {
	c, err := parseFlags(nil, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if c.ConfigDir != "/config" || c.Addr != ":6262" {
		t.Fatalf("defaults wrong: %+v", c)
	}
}

func TestParseFlagsSet(t *testing.T) {
	c, err := parseFlags([]string{"-config", "/data", "-addr", ":9000"}, io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	if c.ConfigDir != "/data" || c.Addr != ":9000" {
		t.Fatalf("flags not applied: %+v", c)
	}
}

func TestParseFlagsRejectsExtra(t *testing.T) {
	if _, err := parseFlags([]string{"stray"}, io.Discard); err == nil {
		t.Fatal("expected an error for stray arguments")
	}
}

func TestConfigDirArg(t *testing.T) {
	for _, c := range []struct {
		args []string
		want string
	}{
		{nil, "/config"},
		{[]string{"-addr", ":6262"}, "/config"},
		{[]string{"-config", "/data", "-addr", ":1"}, "/data"},
		{[]string{"reset-owner", "--config", "/d2"}, "/d2"},
		{[]string{"restore", "-config=/d3", "--list"}, "/d3"},
		{[]string{"import-secrets", "--config=/d4"}, "/d4"},
		{[]string{"doctor", "-json", "-config", "/d5"}, "/d5"},
		{[]string{"-config"}, "/config"},
	} {
		if got := configDirArg(c.args); got != c.want {
			t.Errorf("configDirArg(%q) = %q, want %q", c.args, got, c.want)
		}
	}
}
