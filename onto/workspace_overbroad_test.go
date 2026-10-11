package onto

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mkOverbroadFixture builds a minimal single-or-two-member workspace so each
// test controls exactly one README and measures its owns-emission in isolation.
func mkOverbroadFixture(t *testing.T, docs map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, ".agent-one/policy"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".agent-one/policy/AGENT-ONE.md"), []byte("# stub\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for path, agent := range docs {
		full := filepath.Join(dir, path)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, []byte(agent), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// ownsSet returns the set of pOwns objects emitted for subject in workspace.
func ownsSet(t *testing.T, w *Workspace, subject string) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	for _, tr := range w.Graph.All() {
		if tr.S == Ao(subject) && tr.P == pOwns {
			out[tr.O.Value] = true
		}
	}
	return out
}

// TestDeriveOwns_ScratchSpansDoNotDerive — census class (a): an agent journaling
// its walk of `.agent-one/tmp/gosdk` or writing under `.agent-one/tmp/zone-loop`
// is describing *shared per-run scratch soil*, never owned ownership. The
// prose supplement must refuse `.agent-one/tmp/<x>` spans regardless of self-naming.
func TestDeriveOwns_ScratchSpansDoNotDerive(t *testing.T) {
	dir := mkOverbroadFixture(t, map[string]string{
		".agent-one/zone/workspace-core/README.md": "name: zone-workspace-core\n" +
			"Ownership: `workspace/`\n" +
			"\n" +
			"- The Go SDK surface under `.agent-one/tmp/gosdk` was walked by zone-workspace-core on commission; it is shared scratch.\n" +
			"- zone-workspace-core exercised the workspace fs and left a probe under `.agent-one/tmp/netprobe`.\n",
	})
	w, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	owns := ownsSet(t, w, "zone-workspace-core")
	for _, bad := range []string{".agent-one/tmp/gosdk", ".agent-one/tmp/netprobe", ".agent-one/tmp", ".agent-one"} {
		if owns[bad] {
			t.Errorf("scratch span %q derived as owns of zone-workspace-core", bad)
		}
	}
	if !owns["workspace/"] && !owns["workspace"] {
		t.Errorf("structured Ownership workspace/ was not emitted (owns=%v)", owns)
	}
	// positive: slitscan prose forbidden; plain `workspace` (normalised from `workspace/`) is the structured own.
}

// TestDeriveOwns_BareAgentOneDoesNotDerive — census class (b): lines like
// "the census-derived owns flagged `.agent-one/` billed to THIS zone worker's zone" name
// `.agent-one` as the *site of a mis-attribution*, not an ownership claim. The
// prose supplement must refuse bare `.agent-one` (no subdir) regardless of self-naming.
func TestDeriveOwns_BareAgentOneDoesNotDerive(t *testing.T) {
	dir := mkOverbroadFixture(t, map[string]string{
		".agent-one/zone/app-commands/README.md": "name: zone-app-commands\n" +
			"Ownership: `app/commands.go`\n" +
			"\n" +
			"- 2026-10-04 — the record tool's memo embeds ticked `.agent-one/` inside zone-app-commands's spine.\n" +
			"- zone-app-commands itself notes the census attributes `.agent-one/` writes to this zone worker's zone, none matching `app/*`.\n",
	})
	w, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	owns := ownsSet(t, w, "zone-app-commands")
	if owns[".agent-one"] || owns[".agent-one/"] {
		t.Errorf("bare .agent-one derived as owns of zone-app-commands (owns=%v)", owns)
	}
	if !owns["app/commands.go"] {
		t.Errorf("structured Ownership app/commands.go was not emitted (owns=%v)", owns)
	}
	// positive: ancestor `app` derived from the structured own's parent chain is pre-existing behaviour.
}

// TestDeriveOwns_QueryPlaceholdersDoNotDerive — census class (c): live tree
// leaks `api/plan?agent=<id>` into owns. A path carrying a query template is
// a URI, not a filesystem path; the prose supplement must refuse it.
func TestDeriveOwns_QueryPlaceholdersDoNotDerive(t *testing.T) {
	dir := mkOverbroadFixture(t, map[string]string{
		".agent-one/zone/tui-board/README.md": "name: zone-tui-board\n" +
			"Ownership: `tui/`\n" +
			"\n" +
			"- live board at `api/plan?agent=<id>` — zone-tui-board serves it.\n",
	})
	w, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	owns := ownsSet(t, w, "zone-tui-board")
	for o := range owns {
		if strings.ContainsAny(o, "?<>") {
			t.Errorf("query/placeholder owns leaked: %q", o)
		}
	}
	if !owns["tui"] && !owns["tui/"] {
		t.Errorf("structured Ownership tui/ was not emitted (owns=%v)", owns)
	}
}

// TestDeriveOwns_StructuredForeignPathDoesNotDerive — census class (e):
// a structured Ownership entry that resolves OUTSIDE the claimant's zone
// is a cross-reference, never ownership — mirror of the prose rule at
// onto/workspace_test.go:463. The fixture member whose zone is
// `.agent-one/zone/owna/core` claims `app/cli.go` (foreign-zone file): that
// structured claim must register NO owns (neither `app` nor `app/cli.go`).
// Meanwhile a structured bare filename (`local.go` — the adopted
// workspace_test.go:387 semantics) and a full in-zone path MUST still derive,
// proving only the foreign claim is dropped.
func TestDeriveOwns_StructuredForeignPathDoesNotDerive(t *testing.T) {
	name := "zone-owna"
	docAgent := "name: " + name + "\n" +
		"Ownership: `.agent-one/zone/owna/core`, `.agent-one/zone/owna/inner/deep.go`, `local.go`, `app/cli.go`\n" +
		"\n" +
		name + " touches app/cli.go only as a caller.\n"
	dir := mkOverbroadFixture(t, map[string]string{
		".agent-one/zone/owna/README.md":          docAgent,
		".agent-one/zone/owna/core/inner/deep.go": "package inner\n",
		".agent-one/zone/owna/core/local.go":      "package owna\n",
		"app/cli.go":                              "package main\n",
	})
	w, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	owns := ownsSet(t, w, "zone-owna")
	if owns["app"] || owns["app/cli.go"] {
		t.Errorf("structured foreign claim app/cli.go derived owns — out-of-zone structured claims are cross-references, never soil (owns=%v)", owns)
	}
	if !owns[".agent-one/zone/owna/inner/deep.go"] {
		t.Errorf("full in-zone structured claim was suppressed along with the foreign one (owns=%v)", owns)
	}
	if !owns["local.go"] {
		t.Errorf("bare zone-local filename claim local.go was suppressed — the adopted bare-filename semantics (workspace_test.go:387) MUST still derive (owns=%v)", owns)
	}
}

// TestDeriveOwns_APIRoutesDoNotDerive — census class (d): prose-narrated
// leading-slash spans that hold NO dot-extension (`/api/tree`,
// `/api/tree/children`) are REST routes, not filesystem soil; normPath
// normalised them merely because they hold a slash, and self-naming prose
// beside the member's name registered owns for them (live: zone-workspace-core
// and zone-tui-board READMEs both narrate `/api/tree`). Meanwhile
// catalogued real soil with dots (`bench/board.go:255`) must survive.
func TestDeriveOwns_APIRoutesDoNotDerive(t *testing.T) {
	name := "zone-tui-board"
	docAgent := "name: " + name + "\n" +
		"Ownership: `tui/`\n" +
		"\n" +
		name + " answers the board handlers `/api/tree` and `/api/tree/children`.\n"
	// the spans above sit in backticks with the member name adjoining —
	// exactly the self-naming prose that registers them (live: zone-workspace-core
	// and zone-tui-board READMEs both narrate `/api/tree`).
	dir := mkOverbroadFixture(t, map[string]string{
		".agent-one/zone/tui-board/README.md": docAgent,
	})
	w, err := Load(dir)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	owns := ownsSet(t, w, "zone-tui-board")
	for o := range owns {
		if strings.HasPrefix(o, "/") && !strings.Contains(o, ".") {
			t.Errorf("REST route %q registered as owns of zone-tui-board — leading-slash, dot-less prose spans are API routes, never soil (owns=%v)", o, owns)
		}
	}
	if !owns["tui"] && !owns["tui/"] {
		t.Errorf("structured Ownership tui/ was not emitted (owns=%v)", owns)
	}
}
