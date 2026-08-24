package pathguard

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestResolveWithinWorkspace(t *testing.T) {
	ws := t.TempDir()
	g, err := New(ws)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	cases := []string{
		".github/report.md",
		"coverage.svg",
		"a/b/c/deep.json",
		".",
	}
	for _, c := range cases {
		got, err := g.Resolve(c)
		if err != nil {
			t.Fatalf("Resolve(%q) unexpected error: %v", c, err)
		}
		if rel, _ := filepath.Rel(g.Root(), got); rel == ".." || len(rel) >= 2 && rel[:2] == ".." {
			t.Fatalf("Resolve(%q) escaped: rel=%q", c, rel)
		}
	}
}

func TestResolveTraversalRejected(t *testing.T) {
	ws := t.TempDir()
	g, err := New(ws)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	bad := []string{
		"../escape.md",
		"../../etc/passwd",
		".github/../../outside.json",
		"a/b/../../../c",
	}
	for _, b := range bad {
		if _, err := g.Resolve(b); !errors.Is(err, ErrEscapes) {
			t.Fatalf("Resolve(%q) expected ErrEscapes, got %v", b, err)
		}
	}
}

func TestResolveAbsoluteOutsideRejected(t *testing.T) {
	ws := t.TempDir()
	g, _ := New(ws)

	outside := filepath.Join(os.TempDir(), "definitely-outside-xyz", "f.md")
	if _, err := g.Resolve(outside); !errors.Is(err, ErrEscapes) {
		t.Fatalf("expected ErrEscapes for absolute outside path, got %v", err)
	}
}

func TestResolveEmpty(t *testing.T) {
	g, _ := New(t.TempDir())
	if _, err := g.Resolve("   "); err == nil {
		t.Fatal("expected error for empty path")
	}
}

func TestNewEmptyWorkspace(t *testing.T) {
	if _, err := New(""); err == nil {
		t.Fatal("expected error for empty workspace")
	}
}

// TestNewNonExistentWorkspace covers the EvalSymlinks fallback branch in New:
// when the workspace cannot be resolved, it falls back to the cleaned absolute
// path instead of failing.
func TestNewNonExistentWorkspace(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "does-not-exist", "sub")
	g, err := New(missing)
	if err != nil {
		t.Fatalf("New should fall back for a non-existent workspace: %v", err)
	}
	if g.Root() != filepath.Clean(missing) {
		t.Fatalf("expected root %q, got %q", filepath.Clean(missing), g.Root())
	}
}

// TestSymlinkParentEscape ensures an output path whose parent is a symlink
// pointing outside the workspace is rejected, even if the leaf does not exist.
func TestSymlinkParentEscape(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is unreliable without privileges on Windows")
	}
	ws := t.TempDir()
	outside := t.TempDir()

	// Create ws/link -> outside
	link := filepath.Join(ws, "link")
	if err := os.Symlink(outside, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	g, err := New(ws)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	// ws/link/report.md resolves (via symlink) to outside/report.md.
	if _, err := g.Resolve("link/report.md"); !errors.Is(err, ErrEscapes) {
		t.Fatalf("expected ErrEscapes for symlinked parent, got %v", err)
	}
}

// TestSymlinkInternalAllowed ensures an internal symlink that stays inside the
// workspace is allowed.
func TestSymlinkInternalAllowed(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is unreliable without privileges on Windows")
	}
	ws := t.TempDir()
	if err := os.Mkdir(filepath.Join(ws, "real"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Symlink(filepath.Join(ws, "real"), filepath.Join(ws, "alias")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	g, _ := New(ws)
	if _, err := g.Resolve("alias/report.md"); err != nil {
		t.Fatalf("expected internal symlink to be allowed, got %v", err)
	}
}

func TestResolveCleansDotSegments(t *testing.T) {
	ws := t.TempDir()
	g, _ := New(ws)
	got, err := g.Resolve("./a/./b/report.md")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := filepath.Join(g.Root(), "a", "b", "report.md")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestNewRelativeWorkspace exercises the Abs path in New using a relative
// workspace value.
func TestNewRelativeWorkspace(t *testing.T) {
	ws := t.TempDir()
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(cwd) })
	if err := os.Chdir(ws); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	g, err := New(".")
	if err != nil {
		t.Fatalf("New(.): %v", err)
	}
	got, err := g.Resolve("out/report.md")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	if rel, _ := filepath.Rel(g.Root(), got); rel != filepath.Join("out", "report.md") {
		t.Fatalf("unexpected rel %q", rel)
	}
}

// TestResolveDeepNonExistentToRoot exercises resolveExistingAncestor walking up
// several non-existent segments before hitting an existing ancestor.
func TestResolveDeepNonExistentToRoot(t *testing.T) {
	ws := t.TempDir()
	g, _ := New(ws)
	got, err := g.Resolve("x/y/z/w/report.md")
	if err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	want := filepath.Join(g.Root(), "x", "y", "z", "w", "report.md")
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

// TestResolveBrokenSymlinkParent ensures a dangling symlink in the parent chain
// produces an error rather than silently escaping.
func TestResolveBrokenSymlinkParent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation is unreliable without privileges on Windows")
	}
	ws := t.TempDir()
	// ws/broken -> ws/missing-target (does not exist) => EvalSymlinks fails.
	if err := os.Symlink(filepath.Join(ws, "missing-target"), filepath.Join(ws, "broken")); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	g, _ := New(ws)
	if _, err := g.Resolve("broken/report.md"); err == nil {
		t.Fatal("expected error for broken symlink parent")
	}
}
