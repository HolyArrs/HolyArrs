package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net"
	"strings"
	"testing"

	"github.com/HolyArrs/holyarrs/app/backend/internal/core/database"
	"github.com/HolyArrs/holyarrs/app/backend/internal/metadata"
)

func TestDoctorCmd(t *testing.T) {
	dir := t.TempDir()
	s, err := database.Open(dir, database.Options{})
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close() // the server keeps it open; doctor only reads
	// A closed port: the network check fails fast, so the exit is 1.
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	_ = ln.Close()
	t.Setenv(metadata.EnvServiceURL, "http://"+addr+"/?apikey=NEVER_SHOWN")
	var out bytes.Buffer
	if code := doctorCmd([]string{"-config", dir, "-save-path", t.TempDir()}, &out, io.Discard); code != 1 {
		t.Fatalf("exit %d:\n%s", code, out.String())
	}
	text := out.String()
	for _, want := range []string{"HolyArrs doctor", "Config folder", "downloads ", "Network", "FAIL", "Clock", "Result:"} {
		if !strings.Contains(text, want) {
			t.Errorf("missing %q:\n%s", want, text)
		}
	}
	if strings.Contains(text, "NEVER_SHOWN") {
		t.Fatalf("secret shown:\n%s", text)
	}
	out.Reset()
	if code := doctorCmd([]string{"-config", dir, "-json"}, &out, io.Discard); code != 1 {
		t.Fatalf("json exit %d", code)
	}
	var got struct {
		OK     bool             `json:"ok"`
		Checks []map[string]any `json:"checks"`
	}
	if err := json.Unmarshal(out.Bytes(), &got); err != nil || got.OK || len(got.Checks) < 5 {
		t.Fatalf("%v %s", err, out.String())
	}
	if code := doctorCmd([]string{"stray"}, &out, io.Discard); code != 2 {
		t.Fatalf("stray exit %d", code)
	}
}
