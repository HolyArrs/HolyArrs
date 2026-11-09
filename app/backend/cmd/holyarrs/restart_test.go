package main

import (
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"testing"
	"time"
)

// EnvRestartTiming turns on TestCrashRestartTime; with a file path in
// EnvRestartTimingOut the times are also written there.
const (
	EnvRestartTiming    = "HOLYARRS_RESTART_TIMING"
	EnvRestartTimingOut = "HOLYARRS_RESTART_TIMING_OUT"
)

// TestCrashRestartTime measures the time from a crash (the process killed)
// to a working sign-in page when HolyArrs is started again at once, as a
// restart policy does. It fails when the median is over 2 s or the page
// never comes back.
func TestCrashRestartTime(t *testing.T) {
	if os.Getenv(EnvRestartTiming) == "" {
		t.Skip("set " + EnvRestartTiming + "=1 to measure the crash-to-sign-in-page time")
	}
	dir := t.TempDir()
	bin := filepath.Join(dir, "holyarrs")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, ".")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build: %v\n%s", err, out)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	ln.Close()
	cfg := filepath.Join(dir, "config")
	start := func() *exec.Cmd {
		cmd := testCommand(bin, "-config", cfg, "-addr", addr)
		if err := cmd.Start(); err != nil {
			t.Fatal(err)
		}
		return cmd
	}
	cl := &http.Client{Timeout: 2 * time.Second}
	waitPage := func() time.Duration {
		began := time.Now()
		for time.Since(began) < 60*time.Second {
			if resp, err := cl.Get("http://" + addr + "/login"); err == nil {
				resp.Body.Close()
				if resp.StatusCode == http.StatusOK {
					return time.Since(began)
				}
			}
			time.Sleep(5 * time.Millisecond)
		}
		t.Fatal("the sign-in page did not come back within 60 s")
		return 0
	}
	cmd := start()
	first := waitPage() // first start makes the databases
	var times []time.Duration
	for range 5 {
		time.Sleep(500 * time.Millisecond)
		killed := time.Now()
		_ = cmd.Process.Kill() // a crash: no clean shutdown
		_ = cmd.Wait()
		cmd = start()
		waitPage()
		times = append(times, time.Since(killed))
	}
	_ = cmd.Process.Kill()
	_ = cmd.Wait()
	sorted := append([]time.Duration(nil), times...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })
	line := fmt.Sprintf("%s %s/%s: first start %v; crash to sign-in page over %d runs: median %v, slowest %v (%v)",
		time.Now().UTC().Format(time.RFC3339), runtime.GOOS, runtime.GOARCH, first.Round(time.Millisecond), len(times),
		sorted[len(sorted)/2].Round(time.Millisecond), sorted[len(sorted)-1].Round(time.Millisecond), times)
	t.Log(line)
	// The pass line (3.7.10), from the first measurements: 58 to 70 ms
	// for the process on a fresh config, about 0.6 s (median 584 ms) through Docker's
	// restart policy with the demo library on an image of the 3.7 code
	// (releases/3.7-fault-and-memory.md).
	// A slow CI machine gets room; a regression of the start-up path does not.
	if med := sorted[len(sorted)/2]; med > 2*time.Second {
		t.Errorf("crash to sign-in page: median %v, the line is 2 s", med)
	}
	if out := os.Getenv(EnvRestartTimingOut); out != "" {
		if err := os.WriteFile(out, []byte(line+"\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
}
