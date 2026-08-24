package gotest

import (
	"bufio"
	"encoding/json"
	"io"
	"os"
	"sort"
	"strings"

	"github.com/soulteary/go-test-report-action/internal/model"
)

// maxFailureOutputBytes bounds how much captured output we keep per failing
// test case, so a chatty test cannot blow up memory or the report.
const maxFailureOutputBytes = 4096

// ParseResult is the aggregated outcome of a test run derived from the JSONL
// event stream.
type ParseResult struct {
	Tests    model.Tests
	Packages []PackageResult
	Failures []model.Failure
	// CompileFailed is true if any package failed to build/compile.
	CompileFailed bool
	// Panicked is true if a panic was observed in the output stream.
	Panicked bool
}

// PackageResult holds per-package test aggregation.
type PackageResult struct {
	ImportPath string
	Status     string // one of model.Status* values
	Tests      int
	Failed     int
}

// event is the full `go test -json` event shape we consume.
type event struct {
	Action  string `json:"Action"`
	Package string `json:"Package"`
	Test    string `json:"Test"`
	Output  string `json:"Output"`
}

// ParseFile streams and aggregates a test.jsonl file produced by the runner.
func ParseFile(path string) (ParseResult, error) {
	f, err := os.Open(path)
	if err != nil {
		return ParseResult{}, err
	}
	defer f.Close()
	return Parse(f)
}

// pkgAgg accumulates state for a single package while streaming.
type pkgAgg struct {
	importPath   string
	tests        map[string]bool // top-level test name -> failed?
	seen         map[string]bool // top-level test name -> seen
	skipped      map[string]bool
	failOrder    []string
	failRecorded map[string]bool
	failOutput   map[string]*strings.Builder
	pkgFailed    bool
	pkgCompile   bool
	sawAnyTest   bool
}

func newPkgAgg(importPath string) *pkgAgg {
	return &pkgAgg{
		importPath:   importPath,
		tests:        map[string]bool{},
		seen:         map[string]bool{},
		skipped:      map[string]bool{},
		failRecorded: map[string]bool{},
		failOutput:   map[string]*strings.Builder{},
	}
}

// topLevel returns the parent test name (before the first "/") so subtests
// aggregate under their parent.
func topLevel(test string) string {
	if i := strings.IndexByte(test, '/'); i >= 0 {
		return test[:i]
	}
	return test
}

// Parse streams JSON events from r and aggregates them. It reads line by line
// and never loads the whole stream into memory.
func Parse(r io.Reader) (ParseResult, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	pkgs := map[string]*pkgAgg{}
	order := []string{}
	getPkg := func(importPath string) *pkgAgg {
		p, ok := pkgs[importPath]
		if !ok {
			p = newPkgAgg(importPath)
			pkgs[importPath] = p
			order = append(order, importPath)
		}
		return p
	}

	var res ParseResult

	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var ev event
		if err := json.Unmarshal(line, &ev); err != nil {
			continue // ignore non-event noise
		}
		if ev.Package == "" {
			continue
		}
		p := getPkg(ev.Package)

		if strings.Contains(ev.Output, "panic:") {
			res.Panicked = true
		}

		if ev.Test == "" {
			applyPackageEvent(p, ev)
			continue
		}

		applyTestEvent(p, ev)
	}
	if err := scanner.Err(); err != nil {
		return ParseResult{}, err
	}

	// Finalize aggregation.
	sort.Strings(order)
	for _, importPath := range order {
		finalizePackage(&res, importPath, pkgs[importPath])
	}

	return res, nil
}

// applyPackageEvent handles events that carry no Test name (package-level).
func applyPackageEvent(p *pkgAgg, ev event) {
	switch ev.Action {
	case "fail":
		p.pkgFailed = true
		// A package-level fail with no tests run typically means a
		// build/compile failure.
		if !p.sawAnyTest && (isCompileFailure(ev.Output) || compileHint(p)) {
			p.pkgCompile = true
		}
	case "output":
		if isCompileFailure(ev.Output) && !p.sawAnyTest {
			p.pkgCompile = true
		}
	}
}

// applyTestEvent handles events tied to a specific test.
func applyTestEvent(p *pkgAgg, ev event) {
	top := topLevel(ev.Test)
	switch ev.Action {
	case "run":
		p.sawAnyTest = true
		p.seen[top] = true
	case "pass":
		p.sawAnyTest = true
		if top == ev.Test {
			p.seen[top] = true
			// Only mark passed if not already failed by a subtest.
			if _, failed := p.tests[top]; !failed {
				p.tests[top] = false
			}
		}
	case "fail":
		p.sawAnyTest = true
		p.seen[top] = true
		if !p.tests[top] && !p.failRecorded[top] {
			// first time this top-level test is marked failed
			p.failOrder = append(p.failOrder, top)
			p.failRecorded[top] = true
		}
		p.tests[top] = true
	case "skip":
		p.sawAnyTest = true
		if top == ev.Test {
			p.seen[top] = true
			if _, exists := p.tests[top]; !exists {
				p.skipped[top] = true
			}
		}
	case "output":
		captureOutput(p, top, ev.Output)
	}
}

// captureOutput buffers bounded output keyed by top-level test.
func captureOutput(p *pkgAgg, top, output string) {
	b := p.failOutput[top]
	if b == nil {
		b = &strings.Builder{}
		p.failOutput[top] = b
	}
	appendBounded(b, output)
}

// countTests tallies passed/failed/skipped tests for a package.
func countTests(p *pkgAgg) (passed, failed, skipped int) {
	for name, isFailed := range p.tests {
		switch {
		case isFailed:
			failed++
		case p.skipped[name]:
			skipped++
		default:
			passed++
		}
	}
	for name := range p.skipped {
		if _, ok := p.tests[name]; !ok {
			skipped++
		}
	}
	return passed, failed, skipped
}

// packageStatus derives the reported status for a package.
func packageStatus(p *pkgAgg, failed, total int) (status string, compileFailed bool) {
	switch {
	case p.pkgCompile:
		return model.StatusFail, true
	case failed > 0 || p.pkgFailed:
		return model.StatusFail, false
	case total == 0:
		return model.StatusNoTests, false
	default:
		return model.StatusPass, false
	}
}

// finalizePackage aggregates a single package's results into res.
func finalizePackage(res *ParseResult, importPath string, p *pkgAgg) {
	pr := PackageResult{ImportPath: importPath}

	passed, failed, skipped := countTests(p)

	total := passed + failed + skipped
	pr.Tests = total
	pr.Failed = failed

	status, compileFailed := packageStatus(p, failed, total)
	pr.Status = status
	if compileFailed {
		res.CompileFailed = true
	}

	res.Tests.Total += total
	res.Tests.Passed += passed
	res.Tests.Failed += failed
	res.Tests.Skipped += skipped

	// Record failing cases in deterministic order.
	sort.Strings(p.failOrder)
	for _, name := range p.failOrder {
		out := ""
		if b := p.failOutput[name]; b != nil {
			out = strings.TrimRight(b.String(), "\n")
		}
		res.Failures = append(res.Failures, model.Failure{
			Package: importPath,
			Test:    name,
			Output:  out,
		})
	}

	res.Packages = append(res.Packages, pr)
}

func appendBounded(b *strings.Builder, s string) {
	if b.Len() >= maxFailureOutputBytes {
		return
	}
	remaining := maxFailureOutputBytes - b.Len()
	if len(s) > remaining {
		s = s[:remaining]
	}
	b.WriteString(s)
}

// isCompileFailure detects the characteristic build-failure lines emitted on a
// package's output stream when it cannot be compiled.
func isCompileFailure(out string) bool {
	if out == "" {
		return false
	}
	return strings.Contains(out, "[build failed]") ||
		strings.Contains(out, "build failed") ||
		strings.Contains(out, "[setup failed]") ||
		strings.Contains(out, ": syntax error") ||
		strings.Contains(out, "cannot find package")
}

func compileHint(p *pkgAgg) bool {
	return !p.sawAnyTest
}
