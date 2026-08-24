package gotest

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func writeSampleModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/sample\n\ngo 1.26\n",
		"lib.go": "package sample\n\nfunc Add(a, b int) int { return a + b }\n",
		"lib_test.go": `package sample

import "testing"

func TestAdd(t *testing.T) {
	if Add(1, 2) != 3 {
		t.Fatal("bad")
	}
}
`,
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestRun_Integration(t *testing.T) {
	dir := writeSampleModule(t)
	rawDir := t.TempDir()
	var log bytes.Buffer
	res, err := Run(context.Background(), RunOptions{
		Dir:       dir,
		CoverMode: "set",
		Timeout:   60 * time.Second,
		Packages:  []string{"./..."},
	}, rawDir, &log)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit 0, got %d", res.ExitCode)
	}
	if res.JSONLPath == "" {
		t.Fatal("expected jsonl path")
	}
	if res.CoverProfile == "" {
		t.Fatal("expected coverage profile")
	}
	// Human-readable log should include the passing test output.
	if !strings.Contains(log.String(), "PASS") && !strings.Contains(log.String(), "ok") {
		t.Fatalf("expected human log output, got: %s", log.String())
	}

	// ParseFile should aggregate the run.
	parsed, err := ParseFile(res.JSONLPath)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Tests.Passed != 1 || parsed.Tests.Failed != 0 {
		t.Fatalf("unexpected parse result: %+v", parsed.Tests)
	}
}

// TestRun_RelativeCoverProfileAnchored guards against a regression where a
// relative -coverprofile was resolved against opts.Dir (the `go test` working
// directory) instead of this process's CWD, causing the profile to be written
// to a nested, wrong location and the coverage gate to see no data.
func TestRun_RelativeCoverProfileAnchored(t *testing.T) {
	moduleDir := writeSampleModule(t)

	// Run from a separate process CWD so a relative CoverProfile that is
	// resolved against opts.Dir would land in the wrong place.
	procCWD := t.TempDir()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(procCWD); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
	// On some platforms (notably macOS) TempDir returns a path under a symlink
	// (/var -> /private/var). Run anchors via filepath.Abs, which relies on
	// os.Getwd() and yields the resolved path. Read back the effective CWD so
	// our expected path matches what Run reports.
	effectiveCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	relProfile := filepath.Join("out", "coverage.out")
	var log bytes.Buffer
	res, err := Run(context.Background(), RunOptions{
		Dir:          moduleDir,
		CoverMode:    "set",
		CoverProfile: relProfile,
		Timeout:      60 * time.Second,
		Packages:     []string{"./..."},
	}, filepath.Join(effectiveCWD, "raw"), &log)
	if err != nil {
		t.Fatalf("run error: %v", err)
	}
	if res.CoverProfile == "" {
		t.Fatal("expected coverage profile to be reported")
	}
	if !filepath.IsAbs(res.CoverProfile) {
		t.Fatalf("expected an absolute coverage profile path, got %q", res.CoverProfile)
	}
	// The profile must exist at the path anchored to the process CWD, not
	// nested inside moduleDir.
	if _, err := os.Stat(res.CoverProfile); err != nil {
		t.Fatalf("coverage profile not written to reported path: %v", err)
	}
	wantAbs := filepath.Join(effectiveCWD, relProfile)
	if res.CoverProfile != wantAbs {
		t.Fatalf("coverage profile anchored incorrectly: got %q want %q", res.CoverProfile, wantAbs)
	}
	if _, err := os.Stat(filepath.Join(moduleDir, relProfile)); err == nil {
		t.Fatalf("coverage profile leaked into module dir at %q", filepath.Join(moduleDir, relProfile))
	}
}

func TestRun_FailingTestDoesNotAbort(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/f\n\ngo 1.26\n"), 0o644)
	os.WriteFile(filepath.Join(dir, "f_test.go"), []byte(`package f

import "testing"

func TestFail(t *testing.T) { t.Fatal("nope") }
`), 0o644)
	os.WriteFile(filepath.Join(dir, "f.go"), []byte("package f\n\nfunc F() int { return 1 }\n"), 0o644)

	rawDir := t.TempDir()
	var log bytes.Buffer
	res, err := Run(context.Background(), RunOptions{
		Dir:       dir,
		CoverMode: "set",
		Timeout:   60 * time.Second,
		Packages:  []string{"./..."},
	}, rawDir, &log)
	if err != nil {
		t.Fatalf("run should not error on test failure: %v", err)
	}
	if res.ExitCode == 0 {
		t.Fatal("expected non-zero exit code from failing tests")
	}
	parsed, err := ParseFile(res.JSONLPath)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Tests.Failed != 1 {
		t.Fatalf("expected 1 failed test, got %+v", parsed.Tests)
	}
}

func TestList_Integration(t *testing.T) {
	dir := writeSampleModule(t)
	pkgs, err := List(context.Background(), ListOptions{
		Dir:      dir,
		Patterns: []string{"./..."},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(pkgs) != 1 || pkgs[0].ImportPath != "example.com/sample" {
		t.Fatalf("unexpected packages: %+v", pkgs)
	}
	if pkgs[0].Module == nil || pkgs[0].Module.Path != "example.com/sample" {
		t.Fatalf("expected module path, got %+v", pkgs[0].Module)
	}
}

func TestParseFile_Missing(t *testing.T) {
	if _, err := ParseFile(filepath.Join(t.TempDir(), "nope.jsonl")); err == nil {
		t.Fatal("expected error for missing file")
	}
}

func TestStreamEvents_ForwardsOutputWithTestField(t *testing.T) {
	in := `{"Action":"output","Package":"m","Test":"TestAdd","Output":"=== RUN   TestAdd\n"}` + "\n" +
		`{"Action":"output","Package":"m","Output":"PASS\n"}` + "\n" +
		`{"Action":"pass","Package":"m","Test":"TestAdd","Elapsed":0}` + "\n"
	var jsonl, log bytes.Buffer
	if err := streamEvents(strings.NewReader(in), &jsonl, &log); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log.String(), "PASS") || !strings.Contains(log.String(), "RUN") {
		t.Fatalf("output events not forwarded: %q", log.String())
	}
}

func TestStreamEvents_NonJSONLine(t *testing.T) {
	in := "not-json\n" + `{"Action":"output","Package":"m","Output":"hi\n"}` + "\n"
	var jsonl, log bytes.Buffer
	if err := streamEvents(strings.NewReader(in), &jsonl, &log); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(log.String(), "not-json") {
		t.Fatalf("non-json line should be forwarded: %s", log.String())
	}
	if !strings.Contains(log.String(), "hi") {
		t.Fatalf("output event should be forwarded: %s", log.String())
	}
	if !strings.Contains(jsonl.String(), "not-json") {
		t.Fatalf("raw jsonl should contain all lines: %s", jsonl.String())
	}
}

// failWriter fails writes after allowing okWrites successful writes, letting us
// exercise error paths in writeLine and streamEvents.
type failWriter struct {
	okWrites int
	n        int
}

func (f *failWriter) Write(p []byte) (int, error) {
	if f.n >= f.okWrites {
		return 0, errors.New("write failed")
	}
	f.n++
	return len(p), nil
}

func TestWriteLine(t *testing.T) {
	var buf bytes.Buffer
	n, err := writeLine(&buf, []byte("abc"))
	if err != nil {
		t.Fatal(err)
	}
	if n != 4 || buf.String() != "abc\n" {
		t.Fatalf("unexpected writeLine result: n=%d out=%q", n, buf.String())
	}

	// Fail on the first write (the payload).
	if _, err := writeLine(&failWriter{okWrites: 0}, []byte("abc")); err == nil {
		t.Fatal("expected error when payload write fails")
	}
	// Fail on the second write (the newline).
	if _, err := writeLine(&failWriter{okWrites: 1}, []byte("abc")); err == nil {
		t.Fatal("expected error when newline write fails")
	}
}

func TestStreamEvents_WriteError(t *testing.T) {
	// A JSON line whose raw copy to jsonlOut fails should surface the error.
	in := `{"Action":"output","Package":"m","Output":"hi\n"}` + "\n"
	var log bytes.Buffer
	if err := streamEvents(strings.NewReader(in), &failWriter{okWrites: 0}, &log); err == nil {
		t.Fatal("expected error when jsonl write fails")
	}
	// A non-JSON line whose raw copy fails should also surface the error.
	if err := streamEvents(strings.NewReader("not-json\n"), &failWriter{okWrites: 0}, &log); err == nil {
		t.Fatal("expected error when non-json jsonl write fails")
	}
}

func TestAsExitError(t *testing.T) {
	var target *exec.ExitError
	if asExitError(errors.New("plain"), &target) {
		t.Fatal("plain error should not be an ExitError")
	}
	if target != nil {
		t.Fatal("target should stay nil for non-ExitError")
	}
}
