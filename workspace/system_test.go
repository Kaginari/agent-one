package workspace

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const testSystemManifest = `# a test manifest — comments are inside the yaml subset
name: testdim

above: null

workspaces:
  - name: orchestrator
    path: .
    role: orchestrator
  - name: inner
    path: inner
    role: nested

relations:
  - from: inner
    to: orchestrator
    type: reports
    ask: open
`

// writeManifest lays `.agent-one/system.yaml` at the root.
func writeManifest(t *testing.T, root string) string {
	t.Helper()
	p := filepath.Join(root, ".agent-one", "system.yaml")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(testSystemManifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

// bareRoot returns a temp root guaranteed outside any work-tree: self-init is designed to
// leave work-tree roots alone, so the "inits / stays bare" assertions are only meaningful
// below a bare root. This environment's persistent shell carries TMPDIR inside the orchestrator
// work-tree — run with e.g. TMPDIR=$HOME/tmp when it skips here.
func bareRoot(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if gitInWorkTree(root) {
		t.Skip("tempdir walks up to a work-tree; rerun with e.g. TMPDIR=$HOME/tmp go test ./workspace")
	}
	return root
}

// openBare opens a temp orchestrator root holding only `.agent-one/` (+ what the test adds).
func openBare(t *testing.T, root string) *Workspace {
	t.Helper()
	w, err := Open(root, Default(), Options{})
	if err != nil {
		t.Fatal(err)
	}
	return w
}

func git(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %s in %s: %v\n%s", strings.Join(args, " "), dir, err, out)
	}
	return strings.TrimSpace(string(out))
}

// gitInWorkTree reports `git rev-parse --is-inside-work-tree` at dir: true only when it runs
// clean and answers "true" (an error or "false" both mean no work-tree).
func gitInWorkTree(dir string) bool {
	cmd := exec.Command("git", "rev-parse", "--is-inside-work-tree")
	cmd.Dir = dir
	out, err := cmd.Output()
	return err == nil && strings.TrimSpace(string(out)) == "true"
}

func gitCommit(t *testing.T, dir, file, agent string) string {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, file), []byte(agent), 0o644); err != nil {
		t.Fatal(err)
	}
	git(t, dir, "add", file)
	git(t, dir, "-c", "user.email=t@t", "-c", "user.name=t", "commit", "-m", file)
	return git(t, dir, "rev-parse", "HEAD")
}

// T1: a bare orchestrator root bearing the manifest is versioned at Open, on deterministic main.
func TestSystemSelfInitInitsMainOnBareOrchestratorRoot(t *testing.T) {
	root := bareRoot(t)
	writeManifest(t, root)
	w := openBare(t, root)
	gitDir := filepath.Join(root, ".git")
	if st, err := os.Stat(gitDir); err != nil || !st.IsDir() {
		t.Fatalf("Open must git-init a manifest root: .git: %v", err)
	}
	head, err := os.ReadFile(filepath.Join(gitDir, "HEAD"))
	if err != nil || strings.TrimSpace(string(head)) != "ref: refs/heads/main" {
		t.Fatalf("branch must be deterministic main, .git/HEAD = %q (%v)", head, err)
	}
	if !gitInWorkTree(root) {
		t.Fatal("rev-parse --is-inside-work-tree must answer true after self-init")
	}
	found := false
	for _, n := range w.Notes {
		if strings.Contains(n, "init") {
			found = true
		}
	}
	if !found {
		t.Fatalf("the self-init is recorded in Notes, got %v", w.Notes)
	}
}

// T2: an existing work-tree is left untouched (never re-inited, branch never renamed).
func TestSystemSelfInitLeavesExistingWorkTree(t *testing.T) {
	t.Run("own-branch-survives", func(t *testing.T) {
		root := t.TempDir()
		writeManifest(t, root)
		git(t, root, "init", "-b", "trunk")
		head := gitCommit(t, root, "f.txt", "x")
		openBare(t, root)
		if b := git(t, root, "branch", "--show-current"); b != "trunk" {
			t.Fatalf("branch renamed: %q", b)
		}
		if h := git(t, root, "rev-parse", "HEAD"); h != head {
			t.Fatalf("HEAD moved: %s → %s", head, h)
		}
	})
	// 2b: a root nested inside a parent work-tree is inside one (rev-parse walks up) — no init.
	t.Run("nested-in-parent-worktree", func(t *testing.T) {
		parent := t.TempDir()
		git(t, parent, "init", "-b", "parent")
		gitCommit(t, parent, "p.txt", "p")
		root := filepath.Join(parent, "orchestrator")
		writeManifest(t, root)
		w := openBare(t, root)
		if _, err := os.Stat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
			t.Fatal("nested orchestrator must not gain its own .git")
		}
		if !gitInWorkTree(root) {
			t.Fatal("rev-parse must walk up to the parent work-tree")
		}
		for _, n := range w.Notes {
			if strings.Contains(n, "init") {
				t.Fatalf("no init ran, yet Notes say %q", n)
			}
		}
	})
}

// T3: a nested dir with its own .git is never recursed, absorbed or submoduled.
func TestSystemSelfInitNeverAbsorbsNestedRepos(t *testing.T) {
	parent := bareRoot(t) // no parent work-tree: the soak below proves no absorption
	root := filepath.Join(parent, "orchestrator")
	writeManifest(t, root)
	nested := filepath.Join(root, "nested")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatal(err)
	}
	git(t, nested, "init", "-b", "nested")
	nestedHead := gitCommit(t, nested, "n.txt", "n")

	ran, err := GitSelfInit(root)
	if err != nil || !ran {
		t.Fatalf("GitSelfInit on a bare manifest root: ran=%v err=%v", ran, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "index")); !os.IsNotExist(err) {
		t.Fatal("self-init never stages: the root index must not exist")
	}
	if out := git(t, nested, "rev-parse", "--git-dir"); filepath.Base(out) != ".git" {
		t.Fatalf("nested must resolve to its own git dir, got %q", out)
	}
	if _, err := os.Stat(filepath.Join(root, ".gitmodules")); !os.IsNotExist(err) {
		t.Fatal("no submodule machinery: .gitmodules must not exist")
	}
	if h := git(t, nested, "rev-parse", "HEAD"); h != nestedHead {
		t.Fatalf("nested HEAD moved: %s → %s", nestedHead, h)
	}
	// the soak: a bigger tree under the fresh repo (no ignore seeded — bare init only)
	for _, d := range []string{"a", "b", "c/src"} {
		if err := os.MkdirAll(filepath.Join(root, filepath.FromSlash(d)), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(d), "f.txt"), []byte(d), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := os.Stat(filepath.Join(root, ".git", "index")); !os.IsNotExist(err) {
		t.Fatal("nothing absorbed a nested repo into the root: the index must stay absent")
	}
	if out := git(t, root, "ls-files"); out != "" {
		t.Fatalf("nothing absorbed: root ls-files = %q", out)
	}
}

// T4: a root without the manifest is never auto-versioned.
func TestNoManifestMeansNoInit(t *testing.T) {
	root := bareRoot(t)
	if err := os.MkdirAll(filepath.Join(root, ".agent-one"), 0o755); err != nil {
		t.Fatal(err)
	}
	openBare(t, root)
	if _, err := os.Stat(filepath.Join(root, ".git")); !os.IsNotExist(err) {
		t.Fatal("a root without the manifest must never be auto-versioned")
	}
	if gitInWorkTree(root) {
		t.Fatal("rev-parse must still fail without a manifest")
	}
}

// T5: the manifest reads onto the workspace; a malformed or missing one is a note, never a failed open.
func TestLoadSystemReadsTheManifest(t *testing.T) {
	t.Run("live-shape-parses", func(t *testing.T) {
		root := t.TempDir()
		p := writeManifest(t, root)
		d, err := LoadSystem(p)
		if err != nil {
			t.Fatalf("live manifest shape must parse: %v", err)
		}
		if d.Name != "testdim" || d.Above != "" {
			t.Fatalf("name/above: %+v", d)
		}
		if len(d.Workspaces) != 2 || d.Workspaces[0] != (DimWorkspace{Name: "orchestrator", Path: ".", Role: "orchestrator"}) || d.Workspaces[1].Name != "inner" {
			t.Fatalf("workspaces: %+v", d.Workspaces)
		}
		if len(d.Relations) != 1 || d.Relations[0] != (DimRelation{From: "inner", To: "orchestrator", Type: "reports", Ask: "open"}) {
			t.Fatalf("relations: %+v", d.Relations)
		}
	})
	t.Run("open-carries-the-system", func(t *testing.T) {
		root := t.TempDir()
		writeManifest(t, root)
		w := openBare(t, root)
		if w.Manifest == nil || w.Manifest.Name != "testdim" || len(w.Manifest.Workspaces) != 2 || len(w.Manifest.Relations) != 1 {
			t.Fatalf("Open must load the manifest onto the workspace: %+v", w.Manifest)
		}
	})
	t.Run("malformed-is-a-note-not-a-failed-open", func(t *testing.T) {
		root := t.TempDir()
		writeBrokenManifest(t, root, "\tname: tab-indented\n")
		if _, err := LoadSystem(filepath.Join(root, ".agent-one", "system.yaml")); err == nil {
			t.Fatal("a malformed manifest must error on direct load")
		}
		w, err := Open(root, Default(), Options{})
		if err != nil {
			t.Fatalf("a malformed manifest is a note, never a failed open: %v", err)
		}
		found := false
		for _, n := range w.Notes {
			if strings.Contains(n, "system") {
				found = true
			}
		}
		if !found || w.Manifest != nil {
			t.Fatalf("malformed manifest: system nil + note expected, got %+v / %v", w.Manifest, w.Notes)
		}
	})
	t.Run("missing-is-an-error-never-a-failed-open", func(t *testing.T) {
		d, err := LoadSystem(filepath.Join(t.TempDir(), ".agent-one", "system.yaml"))
		if err == nil || d != nil {
			t.Fatalf("a missing manifest reads as an error: %v", err)
		}
		root := t.TempDir()
		if err := os.MkdirAll(filepath.Join(root, ".agent-one"), 0o755); err != nil {
			t.Fatal(err)
		}
		if _, err := Open(root, Default(), Options{}); err != nil {
			t.Fatalf("no manifest at all is a quiet open: %v", err)
		}
	})
}

func writeBrokenManifest(t *testing.T, root, agent string) {
	t.Helper()
	p := filepath.Join(root, ".agent-one", "system.yaml")
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(agent), 0o644); err != nil {
		t.Fatal(err)
	}
}
