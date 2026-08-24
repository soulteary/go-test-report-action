package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/soulteary/go-test-report-action/internal/config"
	"github.com/soulteary/go-test-report-action/internal/coverage"
	"github.com/soulteary/go-test-report-action/internal/gotest"
	"github.com/soulteary/go-test-report-action/internal/model"
)

func TestDispatch_NoArgs(t *testing.T) {
	var out, errb bytes.Buffer
	if code := dispatch(context.Background(), nil, &out, &errb); code != config.ExitConfigError {
		t.Fatalf("expected config error, got %d", code)
	}
}

func TestDispatch_Unknown(t *testing.T) {
	var out, errb bytes.Buffer
	if code := dispatch(context.Background(), []string{"bogus"}, &out, &errb); code != config.ExitConfigError {
		t.Fatalf("expected config error, got %d", code)
	}
}

func TestDispatch_Help(t *testing.T) {
	var out, errb bytes.Buffer
	if code := dispatch(context.Background(), []string{"--help"}, &out, &errb); code != config.ExitSuccess {
		t.Fatalf("expected success, got %d", code)
	}
}

func TestRunCmd_ConfigError(t *testing.T) {
	var out, errb bytes.Buffer
	// race without atomic => config error. Point at a temp dir so that even if
	// validation regressed we never recursively run tests on this repo.
	dir := t.TempDir()
	code := runCmd(context.Background(), []string{"-race", "-cover-mode=set", "-directory=" + dir}, &out, &errb)
	if code != config.ExitConfigError {
		t.Fatalf("expected config error for race+set, got %d", code)
	}
}

func TestRunCmd_BadTestArgs(t *testing.T) {
	var out, errb bytes.Buffer
	code := runCmd(context.Background(), []string{`-test-args=-run 'unterminated`}, &out, &errb)
	if code != config.ExitConfigError {
		t.Fatalf("expected config error for bad test-args, got %d", code)
	}
}

// writeGoModule creates a minimal passing module for an end-to-end run.
func writeGoModule(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	files := map[string]string{
		"go.mod": "module example.com/sample\n\ngo 1.23\n",
		"lib.go": `package sample

func Add(a, b int) int { return a + b }

func Unused(x int) int {
	if x > 0 {
		return x
	}
	return -x
}
`,
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

func TestExecute_EndToEndPassing(t *testing.T) {
	dir := writeGoModule(t)
	outDir := t.TempDir()
	cfg := &config.Config{
		Directory:      dir,
		Packages:       "./...",
		CoverMode:      config.CoverModeSet,
		JSONOutput:     filepath.Join(outDir, "report.json"),
		MarkdownOutput: filepath.Join(outDir, "report.md"),
		SVGOutput:      filepath.Join(outDir, "badge.svg"),
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := execute(context.Background(), cfg, nil, &out, &errb)
	if code != config.ExitSuccess {
		t.Fatalf("expected success, got %d; stderr=%s", code, errb.String())
	}
	for _, p := range []string{cfg.JSONOutput, cfg.MarkdownOutput, cfg.SVGOutput} {
		if _, err := os.Stat(p); err != nil {
			t.Fatalf("expected report %s: %v", p, err)
		}
	}
	if !bytes.Contains(out.Bytes(), []byte("Test Report")) {
		t.Fatalf("expected job summary on stdout, got: %s", out.String())
	}
}

func TestExecute_CoverageGateFails(t *testing.T) {
	dir := writeGoModule(t)
	outDir := t.TempDir()
	cfg := &config.Config{
		Directory:         dir,
		Packages:          "./...",
		CoverMode:         config.CoverModeSet,
		CoverageThreshold: 100, // Unused() is not covered -> below 100%
		JSONOutput:        filepath.Join(outDir, "report.json"),
		MarkdownOutput:    filepath.Join(outDir, "report.md"),
		SVGOutput:         filepath.Join(outDir, "badge.svg"),
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := execute(context.Background(), cfg, nil, &out, &errb)
	if code != config.ExitTotalCoverage {
		t.Fatalf("expected exit 11, got %d; stderr=%s", code, errb.String())
	}
	// Reports must still be generated before gating.
	if _, err := os.Stat(cfg.JSONOutput); err != nil {
		t.Fatalf("report must be generated before gating: %v", err)
	}
}

func TestStringSlice_SetAndString(t *testing.T) {
	var s stringSlice
	if err := s.Set("a"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := s.Set("b"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := s.String(); got != "a,b" {
		t.Fatalf("expected \"a,b\", got %q", got)
	}
}

func TestDispatch_ValidatePathsSuccess(t *testing.T) {
	ws := t.TempDir()
	var out, errb bytes.Buffer
	code := dispatch(context.Background(), []string{
		"validate-paths",
		"-workspace=" + ws,
		"-path=sub/report.md",
		"-path=badge.svg",
	}, &out, &errb)
	if code != config.ExitSuccess {
		t.Fatalf("expected success, got %d; stderr=%s", code, errb.String())
	}
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if len(lines) != 2 {
		t.Fatalf("expected 2 resolved paths, got %d: %q", len(lines), out.String())
	}
}

func TestDispatch_ValidatePathsEscapeFails(t *testing.T) {
	ws := t.TempDir()
	var out, errb bytes.Buffer
	code := dispatch(context.Background(), []string{
		"validate-paths",
		"-workspace=" + ws,
		"-path=../escape.md",
	}, &out, &errb)
	if code != config.ExitConfigError {
		t.Fatalf("expected config error for path escape, got %d", code)
	}
}

func TestDispatch_ValidatePathsBadFlags(t *testing.T) {
	var out, errb bytes.Buffer
	code := dispatch(context.Background(), []string{
		"validate-paths",
		"-not-a-flag",
	}, &out, &errb)
	if code != config.ExitConfigError {
		t.Fatalf("expected config error for bad flags, got %d", code)
	}
}

func TestDispatch_ValidatePathsEmptyWorkspace(t *testing.T) {
	var out, errb bytes.Buffer
	code := dispatch(context.Background(), []string{
		"validate-paths",
		"-workspace=",
		"-path=report.md",
	}, &out, &errb)
	if code != config.ExitConfigError {
		t.Fatalf("expected config error for empty workspace, got %d", code)
	}
}

func TestRunCmd_BadFlag(t *testing.T) {
	var out, errb bytes.Buffer
	if code := runCmd(context.Background(), []string{"-nope"}, &out, &errb); code != config.ExitConfigError {
		t.Fatalf("expected config error for unknown flag, got %d", code)
	}
}

func TestExecute_ListFails(t *testing.T) {
	// A directory that is not a Go module makes `go list` fail -> toolchain error.
	dir := t.TempDir()
	outDir := t.TempDir()
	cfg := &config.Config{
		Directory:      dir,
		Packages:       "./...",
		CoverMode:      config.CoverModeSet,
		JSONOutput:     filepath.Join(outDir, "report.json"),
		MarkdownOutput: filepath.Join(outDir, "report.md"),
		SVGOutput:      filepath.Join(outDir, "badge.svg"),
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := execute(context.Background(), cfg, nil, &out, &errb)
	if code != config.ExitToolchainError {
		t.Fatalf("expected toolchain error, got %d; stderr=%s", code, errb.String())
	}
}

func TestExecute_PackageGateFails(t *testing.T) {
	dir := writeGoModule(t)
	outDir := t.TempDir()
	cfg := &config.Config{
		Directory:        dir,
		Packages:         "./...",
		CoverMode:        config.CoverModeSet,
		PackageThreshold: 100, // Unused() lowers the single package below 100%
		JSONOutput:       filepath.Join(outDir, "report.json"),
		MarkdownOutput:   filepath.Join(outDir, "report.md"),
		SVGOutput:        filepath.Join(outDir, "badge.svg"),
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := execute(context.Background(), cfg, nil, &out, &errb)
	if code != config.ExitPackageCoverage {
		t.Fatalf("expected exit 12, got %d; stderr=%s", code, errb.String())
	}
}

func TestExecute_SummaryToFile(t *testing.T) {
	dir := writeGoModule(t)
	outDir := t.TempDir()
	summaryPath := filepath.Join(outDir, "summary.md")
	cfg := &config.Config{
		Directory:      dir,
		Packages:       "./...",
		CoverMode:      config.CoverModeSet,
		JSONOutput:     filepath.Join(outDir, "report.json"),
		MarkdownOutput: filepath.Join(outDir, "report.md"),
		SVGOutput:      filepath.Join(outDir, "badge.svg"),
		SummaryOutput:  summaryPath,
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := execute(context.Background(), cfg, nil, &out, &errb)
	if code != config.ExitSuccess {
		t.Fatalf("expected success, got %d; stderr=%s", code, errb.String())
	}
	if _, err := os.Stat(summaryPath); err != nil {
		t.Fatalf("expected summary written to file: %v", err)
	}
	data, err := os.ReadFile(summaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("Test Report")) {
		t.Fatalf("expected summary content in file, got: %s", data)
	}
	// The summary must NOT also be duplicated onto stdout.
	if bytes.Contains(out.Bytes(), []byte("Test Report")) {
		t.Fatalf("summary should not be written to stdout when a file is set, got: %s", out.String())
	}
}

func TestWriteReports_EnsureDirFails(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission semantics differ on Windows")
	}
	// Create a read-only parent so MkdirAll for a nested output path fails.
	base := t.TempDir()
	locked := filepath.Join(base, "locked")
	if err := os.Mkdir(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })

	cfg := &config.Config{
		JSONOutput: filepath.Join(locked, "nested", "report.json"),
	}
	var errb bytes.Buffer
	code := writeReports(cfg, model.Report{}, &errb)
	if code != config.ExitConfigError {
		t.Fatalf("expected config error when dir cannot be created, got %d; stderr=%s", code, errb.String())
	}
}

func TestEnsureDir(t *testing.T) {
	if err := ensureDir("plain.txt"); err != nil {
		t.Fatalf("expected nil for bare filename, got %v", err)
	}
	if err := ensureDir("./plain.txt"); err != nil {
		t.Fatalf("expected nil for ./ prefix, got %v", err)
	}
	dir := t.TempDir()
	nested := filepath.Join(dir, "a", "b", "c.txt")
	if err := ensureDir(nested); err != nil {
		t.Fatalf("expected nested dir created, got %v", err)
	}
	if _, err := os.Stat(filepath.Dir(nested)); err != nil {
		t.Fatalf("expected dir to exist: %v", err)
	}
}

func TestDeriveModulePath(t *testing.T) {
	if got := deriveModulePath(nil); got != "" {
		t.Fatalf("expected empty for nil, got %q", got)
	}
	withModule := gotest.ListedPackage{ImportPath: "example.com/y"}
	withModule.Module = &struct {
		Path string `json:"Path"`
		Dir  string `json:"Dir"`
	}{Path: "example.com"}
	pkgs := []gotest.ListedPackage{
		{ImportPath: "example.com/x"},
		withModule,
	}
	if got := deriveModulePath(pkgs); got != "example.com" {
		t.Fatalf("expected example.com, got %q", got)
	}
}

func TestDecideExitCode(t *testing.T) {
	cfg := &config.Config{}

	// Compile failure takes precedence over everything.
	compile := gotest.ParseResult{CompileFailed: true}
	if got := decideExitCode(cfg, compile, coverage.Result{}, nil); got != config.ExitTestFailure {
		t.Fatalf("compile failure should be exit 10, got %d", got)
	}

	// A failing test also yields a test failure.
	failing := gotest.ParseResult{Tests: model.Tests{Failed: 1}}
	if got := decideExitCode(cfg, failing, coverage.Result{}, nil); got != config.ExitTestFailure {
		t.Fatalf("failing test should be exit 10, got %d", got)
	}

	// Passing tests with no thresholds succeed. Coverage packages not in the
	// included set are treated as excluded from the per-package gate.
	passing := gotest.ParseResult{Tests: model.Tests{Passed: 1}}
	covRes := coverage.Result{
		CoveredStatements: 1,
		TotalStatements:   2,
		Packages: []coverage.PackageCoverage{
			{ImportPath: "m/included", CoveredStatements: 1, TotalStatements: 1},
			{ImportPath: "m/other", CoveredStatements: 0, TotalStatements: 1},
		},
	}
	cfgPkg := &config.Config{PackageThreshold: 100}
	// Only m/included is in scope; m/other is excluded, so the gate passes.
	if got := decideExitCode(cfgPkg, passing, covRes, []string{"m/included"}); got != config.ExitSuccess {
		t.Fatalf("out-of-scope package must not fail the gate, got %d", got)
	}
}

// TestExecute_SummaryWriteFails covers the branch where writing the Job Summary
// to a file fails (parent directory does not exist).
func TestExecute_SummaryWriteFails(t *testing.T) {
	dir := writeGoModule(t)
	outDir := t.TempDir()
	cfg := &config.Config{
		Directory:      dir,
		Packages:       "./...",
		CoverMode:      config.CoverModeSet,
		JSONOutput:     filepath.Join(outDir, "report.json"),
		MarkdownOutput: filepath.Join(outDir, "report.md"),
		SVGOutput:      filepath.Join(outDir, "badge.svg"),
		// Nested, non-existent directory: os.WriteFile cannot create it.
		SummaryOutput: filepath.Join(outDir, "missing-dir", "summary.md"),
	}
	if err := cfg.Validate(); err != nil {
		t.Fatal(err)
	}
	var out, errb bytes.Buffer
	code := execute(context.Background(), cfg, nil, &out, &errb)
	if code != config.ExitToolchainError {
		t.Fatalf("expected toolchain error when summary write fails, got %d; stderr=%s", code, errb.String())
	}
}

// TestWriteReports_WriteFileFails covers the branch where the output directory
// exists but writing the file fails because the target path is a directory.
func TestWriteReports_WriteFileFails(t *testing.T) {
	outDir := t.TempDir()
	// Make JSONOutput a directory so os.WriteFile fails with EISDIR.
	jsonPath := filepath.Join(outDir, "report.json")
	if err := os.Mkdir(jsonPath, 0o755); err != nil {
		t.Fatal(err)
	}
	cfg := &config.Config{JSONOutput: jsonPath}
	var errb bytes.Buffer
	code := writeReports(cfg, model.Report{}, &errb)
	if code != config.ExitToolchainError {
		t.Fatalf("expected toolchain error when file write fails, got %d; stderr=%s", code, errb.String())
	}
}
