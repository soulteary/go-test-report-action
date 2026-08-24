package coverage

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/soulteary/go-test-report-action/internal/config"
)

func writeProfile(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "coverage.out")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestParseProfile_Set(t *testing.T) {
	prof := `mode: set
m/pkga/a.go:1.1,3.2 2 1
m/pkga/a.go:3.2,5.2 2 0
m/pkgb/b.go:1.1,2.2 1 1
`
	res, err := ParseProfile(writeProfile(t, prof))
	if err != nil {
		t.Fatal(err)
	}
	// pkga: 2 covered of 4; pkgb: 1 of 1 => total 3/5.
	if res.CoveredStatements != 3 || res.TotalStatements != 5 {
		t.Fatalf("unexpected totals: %+v", res)
	}
	if got := res.Percentage(); got < 59.9 || got > 60.1 {
		t.Fatalf("expected ~60%%, got %.4f", got)
	}
	if len(res.Packages) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(res.Packages))
	}
}

func TestParseProfile_CountAtomic(t *testing.T) {
	for _, mode := range []string{"count", "atomic"} {
		prof := "mode: " + mode + `
m/p/x.go:1.1,2.2 3 5
m/p/x.go:2.2,3.2 2 0
`
		res, err := ParseProfile(writeProfile(t, prof))
		if err != nil {
			t.Fatalf("%s: %v", mode, err)
		}
		// count>0 => covered. 3 covered of 5.
		if res.CoveredStatements != 3 || res.TotalStatements != 5 {
			t.Fatalf("%s: unexpected totals %+v", mode, res)
		}
	}
}

func TestParseProfile_ZeroStatementPackage(t *testing.T) {
	// A package present but all blocks NumStmt=0 -> N/A.
	prof := `mode: set
m/empty/e.go:1.1,1.1 0 0
m/real/r.go:1.1,2.2 4 4
`
	res, err := ParseProfile(writeProfile(t, prof))
	if err != nil {
		t.Fatal(err)
	}
	var empty, real *PackageCoverage
	for i := range res.Packages {
		switch res.Packages[i].ImportPath {
		case "m/empty":
			empty = &res.Packages[i]
		case "m/real":
			real = &res.Packages[i]
		}
	}
	if empty == nil || !empty.NA {
		t.Fatalf("m/empty should be N/A: %+v", empty)
	}
	if real == nil || real.NA {
		t.Fatalf("m/real should not be N/A: %+v", real)
	}
}

func TestImportPathOf(t *testing.T) {
	if importPathOf("github.com/x/y/pkg/file.go") != "github.com/x/y/pkg" {
		t.Fatal("wrong import path derivation")
	}
	if importPathOf("main.go") != "main.go" {
		t.Fatal("root file should fall back to file name")
	}
}

func TestGate_TotalCoverage(t *testing.T) {
	res := Result{CoveredStatements: 79, TotalStatements: 100}
	cfg := &config.Config{CoverageThreshold: 80}
	if got := Gate(res, cfg, nil); got != config.ExitTotalCoverage {
		t.Fatalf("expected exit 11, got %d", got)
	}
	res.CoveredStatements = 80
	if got := Gate(res, cfg, nil); got != config.ExitSuccess {
		t.Fatalf("expected success at exactly 80%%, got %d", got)
	}
}

func TestGate_TotalBoundaryPrecision(t *testing.T) {
	// 79.995% displayed rounds to 80.00 but raw is below 80 -> must fail.
	res := Result{CoveredStatements: 79995, TotalStatements: 100000}
	cfg := &config.Config{CoverageThreshold: 80}
	if got := Gate(res, cfg, nil); got != config.ExitTotalCoverage {
		t.Fatalf("raw 79.995%% must fail an 80%% gate, got %d", got)
	}
}

func TestGate_PackageThreshold(t *testing.T) {
	res := Result{
		CoveredStatements: 100,
		TotalStatements:   100,
		Packages: []PackageCoverage{
			{ImportPath: "m/good", CoveredStatements: 90, TotalStatements: 100},
			{ImportPath: "m/bad", CoveredStatements: 40, TotalStatements: 100},
			{ImportPath: "m/na", NA: true},
		},
	}
	cfg := &config.Config{CoverageThreshold: 0, PackageThreshold: 70}
	if got := Gate(res, cfg, nil); got != config.ExitPackageCoverage {
		t.Fatalf("expected exit 12 for m/bad, got %d", got)
	}

	// Excluding the bad package should pass.
	excl := map[string]bool{"m/bad": true}
	if got := Gate(res, cfg, excl); got != config.ExitSuccess {
		t.Fatalf("excluded bad package should pass, got %d", got)
	}
}

func TestGate_NAPackageNeverFails(t *testing.T) {
	res := Result{
		CoveredStatements: 10,
		TotalStatements:   10,
		Packages: []PackageCoverage{
			{ImportPath: "m/na", NA: true},
			{ImportPath: "m/ok", CoveredStatements: 10, TotalStatements: 10},
		},
	}
	cfg := &config.Config{PackageThreshold: 90}
	if got := Gate(res, cfg, nil); got != config.ExitSuccess {
		t.Fatalf("N/A packages must never fail the gate, got %d", got)
	}
}

func TestParseProfile_Error(t *testing.T) {
	if _, err := ParseProfile(filepath.Join(t.TempDir(), "missing.out")); err == nil {
		t.Fatal("expected error for missing profile")
	}
}

func TestResultPercentage_Zero(t *testing.T) {
	var r Result
	if r.Percentage() != 0 || r.HasData() {
		t.Fatal("empty result should have 0% and no data")
	}
}

func TestPackageCoveragePercentage(t *testing.T) {
	// Zero total statements (N/A package) reports 0.
	if got := (PackageCoverage{}).Percentage(); got != 0 {
		t.Fatalf("zero-statement package should be 0%%, got %.4f", got)
	}
	// 3 of 4 covered => 75%.
	p := PackageCoverage{CoveredStatements: 3, TotalStatements: 4}
	if got := p.Percentage(); got < 74.99 || got > 75.01 {
		t.Fatalf("expected 75%%, got %.4f", got)
	}
}

func TestBelowThreshold_ZeroTotal(t *testing.T) {
	if belowThreshold(0, 0, 80) {
		t.Fatal("zero-total should never be below threshold")
	}
}

func TestGate_NoDataWithThreshold(t *testing.T) {
	res := Result{}
	cfg := &config.Config{CoverageThreshold: 50}
	if got := Gate(res, cfg, nil); got != config.ExitTotalCoverage {
		t.Fatalf("no coverage data with a threshold should fail, got %d", got)
	}
	cfg.CoverageThreshold = 0
	if got := Gate(res, cfg, nil); got != config.ExitSuccess {
		t.Fatalf("no coverage data with zero threshold should pass, got %d", got)
	}
}
