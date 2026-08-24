// Command gtr runs `go test -json`, aggregates results and coverage,
// and generates deterministic JSON/Markdown/SVG reports plus a Job Summary.
//
// Exit codes (see internal/config):
//
//	0  tests passed and coverage satisfied
//	10 test or compile failure
//	11 total coverage below threshold
//	12 an included package below the per-package threshold
//	20 input/path/config error
//	21 Go toolchain or internal execution error
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"time"

	"github.com/soulteary/go-test-report-action/internal/config"
	"github.com/soulteary/go-test-report-action/internal/coverage"
	"github.com/soulteary/go-test-report-action/internal/gotest"
	"github.com/soulteary/go-test-report-action/internal/model"
	"github.com/soulteary/go-test-report-action/internal/report"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	code := dispatch(ctx, os.Args[1:], os.Stdout, os.Stderr)
	stop()
	os.Exit(code)
}

// dispatch routes subcommands: "run" and "validate-paths".
func dispatch(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(stderr, "usage: gtr run [flags]")
		return config.ExitConfigError
	}
	switch args[0] {
	case "run":
		return runCmd(ctx, args[1:], stdout, stderr)
	case "validate-paths":
		return validatePathsCmd(args[1:], stdout, stderr)
	case "-h", "--help", "help":
		fmt.Fprintln(stdout, "usage: gtr <run|validate-paths> [flags]")
		return config.ExitSuccess
	default:
		fmt.Fprintf(stderr, "unknown subcommand %q\n", args[0])
		return config.ExitConfigError
	}
}

// stringSlice is a repeatable string flag.
type stringSlice []string

func (s *stringSlice) String() string { return strings.Join(*s, ",") }
func (s *stringSlice) Set(v string) error {
	*s = append(*s, v)
	return nil
}

// runFlags holds the parsed flag values for the run subcommand.
type runFlags struct {
	directory         string
	packages          string
	exclude           stringSlice
	race              bool
	coverMode         string
	coverPkg          string
	timeout           time.Duration
	testArgs          string
	coverageThreshold float64
	packageThreshold  float64
	jsonOut           string
	mdOut             string
	svgOut            string
	summaryOut        string
	rawOutputDir      string
	maxFailures       int
	maxPackages       int
	sha               string
	branch            string
	runner            string
}

func runCmd(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("run", flag.ContinueOnError)
	fs.SetOutput(stderr)
	var f runFlags
	fs.StringVar(&f.directory, "directory", ".", "module root directory to run tests in")
	fs.StringVar(&f.packages, "packages", "./...", "package patterns to test (space-separated)")
	fs.Var(&f.exclude, "exclude", "regexp matched against import path to exclude (repeatable)")
	fs.BoolVar(&f.race, "race", false, "enable the race detector (requires cover_mode=atomic)")
	fs.StringVar(&f.coverMode, "cover-mode", config.CoverModeSet, "coverage mode: set|count|atomic")
	fs.StringVar(&f.coverPkg, "cover-pkg", "", "value for go test -coverpkg")
	fs.DurationVar(&f.timeout, "timeout", 10*time.Minute, "go test -timeout value")
	fs.StringVar(&f.testArgs, "test-args", "", "extra shell-like args appended to go test")
	fs.Float64Var(&f.coverageThreshold, "coverage-threshold", 0, "minimum total coverage percentage [0,100]")
	fs.Float64Var(&f.packageThreshold, "package-threshold", 0, "minimum per-package coverage percentage [0,100]; 0 disables")
	fs.StringVar(&f.jsonOut, "json-output", "test-report.json", "path for the deterministic JSON report")
	fs.StringVar(&f.mdOut, "markdown-output", "test-report.md", "path for the deterministic Markdown report")
	fs.StringVar(&f.svgOut, "svg-output", "coverage.svg", "path for the coverage SVG badge")
	fs.StringVar(&f.summaryOut, "summary-output", "", "path for the dynamic Job Summary (empty=stdout)")
	fs.StringVar(&f.rawOutputDir, "raw-output-dir", "", "directory for raw artifacts (test.jsonl, coverage.out); empty=temp dir")
	fs.IntVar(&f.maxFailures, "max-failures", report.DefaultMaxFailures, "max failing cases rendered in Markdown")
	fs.IntVar(&f.maxPackages, "max-packages", report.DefaultMaxPackages, "max package rows rendered in Markdown")
	fs.StringVar(&f.sha, "sha", "", "commit SHA (Job Summary only)")
	fs.StringVar(&f.branch, "branch", "", "branch name (Job Summary only)")
	fs.StringVar(&f.runner, "runner", "", "runner label (Job Summary only)")

	if err := fs.Parse(args); err != nil {
		return config.ExitConfigError
	}

	tokens, err := gotest.Tokenize(f.testArgs)
	if err != nil {
		fmt.Fprintf(stderr, "config error: %v\n", err)
		return config.ExitConfigError
	}

	cfg := &config.Config{
		Directory:         f.directory,
		Packages:          f.packages,
		Exclude:           f.exclude,
		Race:              f.race,
		CoverMode:         f.coverMode,
		CoverPkg:          f.coverPkg,
		Timeout:           f.timeout,
		TestArgs:          f.testArgs,
		CoverageThreshold: f.coverageThreshold,
		PackageThreshold:  f.packageThreshold,
		JSONOutput:        f.jsonOut,
		MarkdownOutput:    f.mdOut,
		SVGOutput:         f.svgOut,
		SummaryOutput:     f.summaryOut,
		RawOutputDir:      f.rawOutputDir,
		MaxFailures:       f.maxFailures,
		MaxPackages:       f.maxPackages,
		SHA:               f.sha,
		Branch:            f.branch,
		Runner:            f.runner,
	}

	if err := cfg.Validate(); err != nil {
		fmt.Fprintf(stderr, "config error: %v\n", err)
		return config.ExitConfigError
	}

	return execute(ctx, cfg, tokens, stdout, stderr)
}

// execute runs the full pipeline and returns the semantic exit code. Reports
// are always generated (even on test failure or insufficient coverage) before
// the code is returned, so downstream consumers still get artifacts.
func execute(ctx context.Context, cfg *config.Config, testArgs []string, stdout, stderr io.Writer) int {
	excludeRes, err := cfg.ExcludeRegexps()
	if err != nil {
		fmt.Fprintf(stderr, "config error: %v\n", err)
		return config.ExitConfigError
	}

	// Package discovery.
	pkgs, err := gotest.List(ctx, gotest.ListOptions{
		Dir:      cfg.Directory,
		Patterns: strings.Fields(cfg.Packages),
		Exclude:  excludeRes,
	})
	if err != nil {
		fmt.Fprintf(stderr, "toolchain error: %v\n", err)
		return config.ExitToolchainError
	}

	modulePath := deriveModulePath(pkgs)
	included := make([]string, 0, len(pkgs))
	for _, p := range pkgs {
		included = append(included, p.ImportPath)
	}

	// Raw output dir (non-deterministic artifacts live here).
	rawDir := cfg.RawOutputDir
	cleanup := func() {}
	if rawDir == "" {
		tmp, terr := os.MkdirTemp("", "gtr-*")
		if terr != nil {
			fmt.Fprintf(stderr, "toolchain error: %v\n", terr)
			return config.ExitToolchainError
		}
		rawDir = tmp
		cleanup = func() { _ = os.RemoveAll(tmp) }
	}
	defer cleanup()

	runRes, err := gotest.Run(ctx, gotest.RunOptions{
		Dir:       cfg.Directory,
		CoverMode: cfg.CoverMode,
		Timeout:   cfg.Timeout,
		Race:      cfg.Race,
		CoverPkg:  cfg.CoverPkg,
		ExtraArgs: testArgs,
		Packages:  strings.Fields(cfg.Packages),
	}, rawDir, stdout)
	if err != nil {
		fmt.Fprintf(stderr, "toolchain error: %v\n", err)
		return config.ExitToolchainError
	}

	// Parse test events.
	parseRes, err := gotest.ParseFile(runRes.JSONLPath)
	if err != nil {
		fmt.Fprintf(stderr, "internal error: %v\n", err)
		return config.ExitToolchainError
	}

	// Parse coverage (if a profile was produced).
	var covRes coverage.Result
	if runRes.CoverProfile != "" {
		covRes, err = coverage.ParseProfile(runRes.CoverProfile)
		if err != nil {
			fmt.Fprintf(stderr, "internal error: parse coverage: %v\n", err)
			return config.ExitToolchainError
		}
	}

	// Build deterministic report and write all outputs BEFORE gating.
	rep := report.Build(report.BuildInput{
		Parse:      parseRes,
		Coverage:   covRes,
		Threshold:  cfg.CoverageThreshold,
		ModulePath: modulePath,
	})

	if code := writeReports(cfg, rep, stderr); code != config.ExitSuccess {
		return code
	}

	// Determine semantic exit code.
	exitCode := decideExitCode(cfg, parseRes, covRes, included)

	// Job Summary (dynamic, allowed to include runtime metadata).
	summary := report.Summary(rep, report.SummaryMeta{
		SHA:      cfg.SHA,
		Branch:   cfg.Branch,
		Runner:   cfg.Runner,
		Elapsed:  runRes.Elapsed,
		ExitCode: exitCode,
	})
	if cfg.SummaryOutput == "" {
		_, _ = stdout.Write(summary)
	} else if err := os.WriteFile(cfg.SummaryOutput, summary, 0o644); err != nil {
		fmt.Fprintf(stderr, "internal error: write summary: %v\n", err)
		return config.ExitToolchainError
	}

	return exitCode
}

// decideExitCode maps results to the semantic exit code. Test/compile failures
// take precedence over coverage gates.
func decideExitCode(cfg *config.Config, parseRes gotest.ParseResult, covRes coverage.Result, included []string) int {
	if parseRes.CompileFailed || parseRes.Tests.Failed > 0 {
		return config.ExitTestFailure
	}

	excluded := map[string]bool{}
	includedSet := map[string]bool{}
	for _, p := range included {
		includedSet[p] = true
	}
	// Coverage packages not in the included set are treated as excluded from
	// the per-package gate.
	for _, pc := range covRes.Packages {
		if !includedSet[pc.ImportPath] {
			excluded[pc.ImportPath] = true
		}
	}

	return coverage.Gate(covRes, cfg, excluded)
}

// writeReports writes the deterministic JSON/Markdown/SVG outputs.
func writeReports(cfg *config.Config, rep model.Report, stderr io.Writer) int {
	jsonBytes, err := report.JSON(rep)
	if err != nil {
		fmt.Fprintf(stderr, "internal error: render json: %v\n", err)
		return config.ExitToolchainError
	}
	mdBytes := report.Markdown(rep, report.MarkdownOptions{
		MaxFailures: cfg.MaxFailures,
		MaxPackages: cfg.MaxPackages,
		JSONPath:    filepath.Base(cfg.JSONOutput),
	})
	svgBytes := report.SVG(report.SVGOptions{
		HasData:    rep.Coverage.TotalStatements > 0,
		Percentage: rep.Coverage.Percentage,
	})

	for _, w := range []struct {
		path string
		data []byte
	}{
		{cfg.JSONOutput, jsonBytes},
		{cfg.MarkdownOutput, mdBytes},
		{cfg.SVGOutput, svgBytes},
	} {
		if w.path == "" {
			continue
		}
		if err := ensureDir(w.path); err != nil {
			fmt.Fprintf(stderr, "path error: %v\n", err)
			return config.ExitConfigError
		}
		if err := os.WriteFile(w.path, w.data, 0o644); err != nil {
			fmt.Fprintf(stderr, "internal error: write %s: %v\n", w.path, err)
			return config.ExitToolchainError
		}
	}
	return config.ExitSuccess
}

// deriveModulePath returns the module path from the listed packages, if known.
func deriveModulePath(pkgs []gotest.ListedPackage) string {
	for _, p := range pkgs {
		if p.Module != nil && p.Module.Path != "" {
			return p.Module.Path
		}
	}
	return ""
}

func ensureDir(path string) error {
	dir := filepath.Dir(path)
	if dir == "" || dir == "." {
		return nil
	}
	return os.MkdirAll(dir, 0o755)
}
