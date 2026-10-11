package app

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/onto"
)

// project writes a fixture tree: path → content ("" writes a placeholder line).
func project(t *testing.T, files map[string]string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), "proj")
	for f, agent := range files {
		p := filepath.Join(root, f)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if agent == "" {
			agent = "x\n"
		}
		if err := os.WriteFile(p, []byte(agent), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func tree(t *testing.T, files ...string) string {
	t.Helper()
	m := map[string]string{}
	for _, f := range files {
		m[f] = ""
	}
	return project(t, m)
}

// goFile is a Go source file of about n lines: a doc, imports, exported funcs.
func goFile(pkg, doc string, n int, imports ...string) string {
	var b strings.Builder
	if doc != "" {
		fmt.Fprintf(&b, "// Package %s %s\n", pkg, doc)
	}
	fmt.Fprintf(&b, "package %s\n\n", pkg)
	for _, im := range imports {
		fmt.Fprintf(&b, "import %q\n", im)
	}
	for i := 0; b.Len() == 0 || strings.Count(b.String(), "\n") < n; i++ {
		fmt.Fprintf(&b, "\n// F%d does a thing.\nfunc F%d() int {\n\treturn %d\n}\n", i, i, i)
	}
	if pkg == "main" {
		b.WriteString("\nfunc main() {}\n")
	}
	return b.String()
}

func goTest(pkg string) string {
	return "package " + pkg + "\n\nimport \"testing\"\n\nfunc TestIt(t *testing.T) {}\n"
}

// pad is n lines of filler for a non-Go source file.
func pad(prefix string, n int) string {
	var b strings.Builder
	for i := 0; i < n; i++ {
		fmt.Fprintf(&b, "%s%d\n", prefix, i)
	}
	return b.String()
}

// sketchAt founds a medium team at root and returns the sketch and the write result.
func sketchAt(t *testing.T, root string) (sketch, sketchWrite) {
	t.Helper()
	if _, err := Init("agent-one", root); err != nil {
		t.Fatal(err)
	}
	d, _ := parseDist("agent-one")
	lex := lexiconFor()
	s := sketchTeam(root, d)
	res, err := writeSketch(root, d, s, lex.Ranks, lex.Prefixes, lex.Display)
	if err != nil {
		t.Fatal(err)
	}
	return s, res
}

// sound asserts the team the ontology reads is whole: no finding, nothing housekeep would
// rewire, every claim derives under its member's name, the ranks' fan-out bounded.
func sound(t *testing.T, root string, s sketch) {
	t.Helper()
	w, err := onto.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if f := w.Validate(); len(f) != 0 {
		t.Fatalf("onto check: %v\n%s", f, s.table(lexiconFor().Prefixes))
	}
	rep, left, err := onto.Housekeep(root, false, false)
	if err != nil || len(rep.DeadBonds) != 0 || len(left) != 0 {
		t.Fatalf("housekeep would touch the sketch: dead bonds %v, left %v, %v", rep.DeadBonds, left, err)
	}
	for _, p := range s.all() {
		for _, c := range p.Ownership {
			if !zoneOK(p.Short, c) {
				t.Errorf("%s-%s claims %s, which the ontology does not derive under that name", p.Rank, p.Short, c)
			}
		}
		if p.Rank == "zone" {
			for _, c := range p.Ownership {
				if strings.Contains(c, "/") && !strings.HasSuffix(c, "/") {
					t.Errorf("zone-%s claims the nested file %s: ownerships are directories", p.Short, c)
				}
			}
		}
	}
	for _, o := range s.Domains {
		if len(o.Zones) == 0 {
			t.Errorf("domain-%s carries no zone worker", o.Short)
		}
	}
	if len(s.Coords) > 1 {
		for _, e := range s.Coords[1:] {
			if n := len(e.Domains); n == 0 || n > domainsPerCoord {
				t.Errorf("lane coord-%s carries %d domain owners", e.Short, n)
			}
		}
	}
}

func byName(s sketch) map[string]plan {
	m := map[string]plan{}
	for _, p := range s.all() {
		m[p.Rank+"-"+p.Short] = p
	}
	return m
}

func readDoc(t *testing.T, root, rank, short string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root, ".agent-one", rank, short, "README.md"))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// A small Go module: one coordinator, one domain owner, a zone worker per package dir plus the root's own files;
// verify lines only where tests are.
func TestMediumSmallGoModule(t *testing.T) {
	root := project(t, map[string]string{
		"go.mod":                         "module example.com/tiny\n\ngo 1.22\n",
		"README.md":                      "# tiny\n",
		"main.go":                        goFile("main", "", 30, "example.com/tiny/internal/greet"),
		"internal/greet/greet.go":        goFile("greet", "says hello to whoever asks.", 140),
		"internal/greet/greet_test.go":   goTest("greet"),
		"internal/store/store.go":        goFile("store", "keeps the greetings on disk.", 160, "example.com/tiny/internal/greet"),
		"vendor/x/y.go":                  goFile("y", "", 400),
		"internal/store/testdata/fix.go": goFile("fix", "", 400),
	})
	s, res := sketchAt(t, root)
	sound(t, root, s)
	if len(s.Coords) != 1 || len(s.Domains) != 1 || s.Service != nil {
		t.Fatalf("a small module is one coordinator and one domain owner, no service owner:\n%s", s.table(lexiconFor().Prefixes))
	}
	m := byName(s)
	greet, store, entry := m["zone-internal-greet"], m["zone-internal-store"], m["zone-proj"]
	if len(greet.Ownership) != 1 || greet.Ownership[0] != "internal/greet/" || len(store.Ownership) != 1 || store.Ownership[0] != "internal/store/" {
		t.Fatalf("one zone worker per package dir: %+v / %+v", greet.Ownership, store.Ownership)
	}
	if strings.Join(entry.Ownership, ",") != "main.go" {
		t.Fatalf("the root's own files are the entry zone worker's: %+v", entry.Ownership)
	}
	if len(greet.Verify) != 1 || greet.Verify[0] != "go test ./internal/greet/..." {
		t.Errorf("a tested package verifies with go test: %v", greet.Verify)
	}
	if len(store.Verify) != 0 || len(entry.Verify) != 0 {
		t.Errorf("no tests, no verify line: store %v entry %v", store.Verify, entry.Verify)
	}
	if e := s.Coords[0]; strings.Join(e.Verify, ",") != "go build ./...,go vet ./..." || !contains(e.Ownership, "go.mod") || !contains(e.Ownership, "README.md") {
		t.Errorf("the coordinator holds the manifests and builds the module: %+v", e)
	}
	for _, p := range s.all() {
		for _, c := range p.Ownership {
			if strings.HasPrefix(c, "vendor") || strings.Contains(c, "testdata") {
				t.Errorf("vendored and fixture dirs are no one's: %s", c)
			}
		}
	}
	doc := readDoc(t, root, "zone", "internal-greet")
	for _, want := range []string{"- **Rank:** Zone worker", "- **Owns:** `internal/greet/`", "- **Reports to:** domain-", "- **Skills:** none",
		"says hello to whoever asks", "- sketch: Go package greet: 1 source file, 1 test file", "- sketch: Exports F0, F1", "Imported by zone-",
		"## Verify\n- `go test ./internal/greet/...`"} {
		if !strings.Contains(doc, want) {
			t.Errorf("zone worker doc lacks %q:\n%s", want, doc)
		}
	}
	if doc := readDoc(t, root, "zone", "internal-store"); !strings.Contains(doc, "No tests found in this ownership") || strings.Contains(doc, "## Verify") {
		t.Errorf("an untested zone worker says so and carries no verify:\n%s", doc)
	}
	if doc := readDoc(t, root, "zone", "proj"); !strings.Contains(doc, "Entry point: main.go") {
		t.Errorf("the entry point is observed:\n%s", doc)
	}
	if len(res.wrote) != len(s.all()) || len(res.kept) != 0 {
		t.Errorf("a fresh workspace writes every member: %+v", res)
	}
}

// A larger multi-package Go repository: imports cluster the packages into domain owners, lane coordinators and
// a design coordinator above them, a service owner for the CI; a tiny cmd/ folds into what it imports.
func TestMediumMultiPackageGo(t *testing.T) {
	const mod = "example.com/big/"
	files := map[string]string{
		"go.mod":                   "module example.com/big\n",
		"go.sum":                   "",
		"README.md":                "# big\n",
		"LICENSE":                  "",
		"docs/guide.md":            "",
		".github/workflows/ci.yml": "",
		"Dockerfile":               "",
		"cmd/big/main.go":          goFile("main", "", 10, mod+"app"),
	}
	add := func(pkg string, tested bool, imports ...string) {
		var ims []string
		for _, im := range imports {
			ims = append(ims, mod+im)
		}
		files[pkg+"/"+filepath.Base(pkg)+".go"] = goFile(filepath.Base(pkg), "is the "+pkg+" part.", 300, ims...)
		files[pkg+"/more.go"] = goFile(filepath.Base(pkg), "", 200, ims...)
		if tested {
			files[pkg+"/"+filepath.Base(pkg)+"_test.go"] = goTest(filepath.Base(pkg))
		}
	}
	add("loop", true, "provider", "wire", "compact")
	add("provider", true, "wire")
	add("wire", true)
	add("compact", false, "wire", "provider")
	add("guard", true)
	add("gate", true, "guard")
	add("sandbox", true, "guard")
	add("shell", false, "sandbox", "guard")
	add("tool", true, "shell", "sandbox", "gate", "guard")
	add("tui", true)
	add("web", true, "tui")
	add("board", false, "web", "tui")
	add("config", true)
	add("discover", true, "config")
	add("app", true, "loop", "tool", "tui", "web", "board", "config", "discover", "provider")
	root := project(t, files)
	s, _ := sketchAt(t, root)
	sound(t, root, s)
	m := byName(s)
	if len(s.Coords) < 3 || s.Coords[0].Short != "design" {
		t.Fatalf("a large project gets a design coordinator and lane coordinators:\n%s", s.table(lexiconFor().Prefixes))
	}
	if c := s.Coords[0]; !contains(c.Ownership, "docs/") || !contains(c.Ownership, "go.mod") || len(c.Domains) != len(s.Coords)-1 {
		t.Errorf("the design coordinator holds docs and manifests and speaks for the lanes: %+v", c)
	}
	if app := m["zone-app"]; strings.Join(app.Ownership, ",") != "app/,cmd/" {
		t.Errorf("tiny cmd/ folds into app/, which it imports: %+v", app.Ownership)
	}
	domainOf := map[string]string{}
	for _, o := range s.Domains {
		if len(o.Zones) > maxPerDomain {
			t.Errorf("domain-%s carries %d zone workers", o.Short, len(o.Zones))
		}
		for _, sl := range o.Zones {
			domainOf[sl.Short] = o.Short
		}
	}
	for _, pair := range [][2]string{{"loop", "provider"}, {"provider", "wire"}, {"gate", "guard"}, {"sandbox", "shell"}, {"tui", "web"}, {"config", "discover"}} {
		if domainOf[pair[0]] != domainOf[pair[1]] {
			t.Errorf("%s and %s are bound by imports but split: %v", pair[0], pair[1], domainOf)
		}
	}
	if domainOf["loop"] == domainOf["tui"] || domainOf["guard"] == domainOf["tui"] {
		t.Errorf("unrelated packages share a domain owner: %v", domainOf)
	}
	if k := s.Service; k == nil || !contains(k.Ownership, ".github/") || !contains(k.Ownership, "Dockerfile") || k.Short != "ci" {
		t.Errorf("CI and a Dockerfile make a service-ci: %+v", s.Service)
	}
	if v := m["zone-shell"].Verify; len(v) != 0 {
		t.Errorf("shell has no tests: %v", v)
	}
	if v := m["zone-wire"].Verify; strings.Join(v, "") != "go test ./wire/..." {
		t.Errorf("wire is tested: %v", v)
	}
	for _, o := range s.Domains {
		for _, v := range o.Verify {
			if strings.Count(v, "go test") != 1 {
				t.Errorf("a domain owner folds its zone workers' go tests into one line: %q", v)
			}
		}
	}
}

// A JS workspace: one zone worker per workspace package, named so the ontology derives each claim,
// verify lines only for packages with a real test script.
func TestMediumJSWorkspace(t *testing.T) {
	root := project(t, map[string]string{
		"package.json":                     `{"name":"acme","private":true,"workspaces":["packages/*","apps/*"]}`,
		"README.md":                        "",
		"packages/ui/package.json":         `{"name":"@acme/ui","description":"The design system's components.","scripts":{"test":"vitest run"}}`,
		"packages/ui/src/index.ts":         "export function Button() {}\n" + pad("// ", 200),
		"packages/ui/src/button.test.ts":   "test('x', () => {})\n",
		"packages/core/package.json":       `{"name":"@acme/core","scripts":{"test":"vitest run"},"dependencies":{"@acme/ui":"*"}}`,
		"packages/core/src/index.ts":       "import { Button } from '@acme/ui'\nexport const core = 1\n" + pad("// ", 200),
		"packages/core/src/core.test.ts":   "test('x', () => {})\n",
		"packages/config/package.json":     `{"name":"@acme/config","scripts":{"test":"echo \"Error: no test specified\" && exit 1"}}`,
		"packages/config/index.js":         "module.exports = {}\n",
		"apps/web/package.json":            `{"name":"web","scripts":{"test":"vitest run"},"dependencies":{"@acme/ui":"*","@acme/core":"*"}}`,
		"apps/web/src/main.tsx":            "import { core } from '@acme/core'\n" + pad("// ", 300),
		"apps/web/src/main.test.tsx":       "test('x', () => {})\n",
		"packages/ui/node_modules/x/i.js":  pad("// ", 500),
		"packages/ui/dist/index.js":        pad("// ", 500),
		"apps/web/src/generated/schema.ts": "// Code generated by x. DO NOT EDIT.\n" + pad("// ", 500),
	})
	s, _ := sketchAt(t, root)
	sound(t, root, s)
	m := byName(s)
	for name, want := range map[string]string{"packages-ui": "npm test --workspace packages/ui", "packages-core": "npm test --workspace packages/core", "apps-web": "npm test --workspace apps/web"} {
		p, ok := m["zone-"+name]
		if !ok || strings.Join(p.Verify, "|") != want {
			t.Errorf("zone-%s verify %v, want %q\n%s", name, p.Verify, want, s.table(lexiconFor().Prefixes))
		}
	}
	if p, ok := m["zone-packages-config"]; !ok || len(p.Verify) != 0 {
		t.Errorf("npm's placeholder test script is no verify line: %+v", p)
	}
	if !strings.Contains(m["zone-packages-ui"].Purpose, "design system's components") {
		t.Errorf("the purpose comes from the package's description: %q", m["zone-packages-ui"].Purpose)
	}
	if o := m["domain-packages"]; len(o.Zones) != 3 {
		t.Errorf("the packages share a domain owner named from their root: %+v", o)
	}
	if !strings.Contains(strings.Join(m["zone-apps-web"].Invariants, " "), "Imports from zone-packages-core, zone-packages-ui") {
		t.Errorf("workspace dependencies are bonds: %v", m["zone-apps-web"].Invariants)
	}
	if !strings.Contains(strings.Join(m["zone-apps-web"].Invariants, " "), "1 source file, 1 test file") {
		t.Errorf("generated code is not counted: %v", m["zone-apps-web"].Invariants)
	}
}

// A Python package: the package and its tests are zone workers; pytest only where test files are.
func TestMediumPythonPackage(t *testing.T) {
	root := project(t, map[string]string{
		"pyproject.toml":            "[project]\nname = \"acme\"\n",
		"README.md":                 "",
		"src/acme/__init__.py":      "\"\"\"Acme turns invoices into ledgers.\"\"\"\n",
		"src/acme/core/__init__.py": "",
		"src/acme/core/engine.py":   "from acme.io import reader\n\ndef run():\n    pass\n" + pad("# ", 200),
		"src/acme/io/__init__.py":   "",
		"src/acme/io/reader.py":     "class Reader:\n    pass\n" + pad("# ", 200),
		"tests/conftest.py":         "",
		"tests/test_engine.py":      "from acme.core import engine\n\ndef test_run():\n    engine.run()\n" + pad("# ", 150),
		".venv/lib/x.py":            pad("# ", 900),
	})
	s, _ := sketchAt(t, root)
	sound(t, root, s)
	m := byName(s)
	acme, tests := m["zone-acme"], m["zone-tests"]
	if strings.Join(acme.Ownership, ",") != "src/acme/" || strings.Join(tests.Ownership, ",") != "tests/" {
		t.Fatalf("the package and its tests:\n%s", s.table(lexiconFor().Prefixes))
	}
	if !strings.Contains(acme.Purpose, "Acme turns invoices into ledgers") {
		t.Errorf("the package docstring is the purpose: %q", acme.Purpose)
	}
	if len(acme.Verify) != 0 || strings.Join(tests.Verify, "") != "python -m pytest -q tests" {
		t.Errorf("pytest only where tests are: acme %v tests %v", acme.Verify, tests.Verify)
	}
	if !strings.Contains(strings.Join(tests.Invariants, " "), "Imports from zone-acme") {
		t.Errorf("the tests' imports bind them to the package: %v", tests.Invariants)
	}
	for _, p := range s.all() {
		if strings.Contains(strings.Join(p.Ownership, ","), ".venv") {
			t.Errorf("a virtualenv is no one's: %+v", p)
		}
	}
}

// A mixed repo with CI, release tooling and infra: the service owner holds them, never a zone worker; docs
// stay with the coordinator; each language gets its own test runner.
func TestMediumMixedRepoWithCI(t *testing.T) {
	root := project(t, map[string]string{
		"go.mod":                        "module example.com/mixed\n",
		"package.json":                  `{"scripts":{"test":"jest"}}`,
		"README.md":                     "",
		"CHANGELOG.md":                  "",
		".goreleaser.yaml":              "",
		".github/workflows/release.yml": "",
		"Dockerfile":                    "",
		"deploy/k8s.yaml":               "",
		"deploy/hook.py":                pad("# ", 200),
		"docs/index.md":                 "",
		"docs/site.js":                  pad("// ", 300),
		"server/server.go":              goFile("server", "serves the API.", 300),
		"server/server_test.go":         goTest("server"),
		"web/app.js":                    "export function App() {}\n" + pad("// ", 300),
		"web/app.test.js":               "test('x', () => {})\n",
		"scripts/migrate.py":            "def migrate():\n    pass\n" + pad("# ", 200),
	})
	s, _ := sketchAt(t, root)
	sound(t, root, s)
	m := byName(s)
	k := s.Service
	if k == nil || k.Short != "release" {
		t.Fatalf("CI with release tooling is service-release: %+v", k)
	}
	for _, want := range []string{".github/", ".goreleaser.yaml", "CHANGELOG.md", "Dockerfile", "deploy/"} {
		if !contains(k.Ownership, want) {
			t.Errorf("service owner lacks %s: %v", want, k.Ownership)
		}
	}
	for _, p := range s.all() {
		if p.Rank == "zone" && (contains(p.Ownership, "deploy/") || contains(p.Ownership, "docs/")) {
			t.Errorf("infra and docs are never a zone worker's: %+v", p)
		}
	}
	if !contains(s.Coords[0].Ownership, "docs/") {
		t.Errorf("docs are the coordinator's: %v", s.Coords[0].Ownership)
	}
	if v := strings.Join(m["zone-server"].Verify, ""); v != "go test ./server/..." {
		t.Errorf("server: %q", v)
	}
	if v := strings.Join(m["zone-web"].Verify, ""); v != "npx jest web" {
		t.Errorf("web is scoped through the root's jest: %q", v)
	}
	if v := m["zone-scripts"].Verify; len(v) != 0 {
		t.Errorf("scripts has no tests: %v", v)
	}
	if !strings.Contains(strings.Join(k.Invariants, " "), "release.yml") {
		t.Errorf("the service owner names the workflows it saw: %v", k.Invariants)
	}
}

// Medium is idempotent and safe: a second run writes nothing, a refined doc is never touched,
// and over a team someone already cut it adds only what nobody holds.
func TestMediumIdempotentAndSafe(t *testing.T) {
	root := project(t, map[string]string{
		"go.mod":            "module example.com/m\n",
		"api/api.go":        goFile("api", "serves.", 300),
		"store/store.go":    goFile("store", "stores.", 300),
		"jobs/jobs.go":      goFile("jobs", "runs jobs.", 300),
		"jobs/jobs_test.go": goTest("jobs"),
	})
	s, first := sketchAt(t, root)
	sound(t, root, s)
	refined := filepath.Join(root, ".agent-one", "zone", "store", "README.md")
	mine := "# zone-store\n\n- **Rank:** Zone worker\n- **Owns:** `store/`\n- **Reports to:** " + s.Domains[0].Zones[0].Parent + "\n- **Purpose:** mine\n\n## Invariants\n- refined by hand\n"
	if err := os.WriteFile(refined, []byte(mine), 0o644); err != nil {
		t.Fatal(err)
	}
	_, again := sketchAt(t, root)
	if len(again.wrote) != 0 || len(again.kept) != len(first.wrote) {
		t.Fatalf("a re-run writes nothing and keeps all: %+v (first %d)", again, len(first.wrote))
	}
	if b, _ := os.ReadFile(refined); string(b) != mine {
		t.Fatalf("a refined doc was touched:\n%s", b)
	}
	// a workspace whose own team already holds api/ under another name: the sketch's zone-api
	// is left out (covered), the rest is added, and the whole still checks clean
	root2 := project(t, map[string]string{
		"go.mod":         "module example.com/m\n",
		"api/api.go":     goFile("api", "serves.", 300),
		"store/store.go": goFile("store", "stores.", 300),
	})
	if _, err := Init("agent-one", root2); err != nil {
		t.Fatal(err)
	}
	for p, agent := range map[string]string{
		".agent-one/coord/team/README.md":  "# coord-team\n\n- **Rank:** Coordinator\n- **Reports to:** orchestrator\n- **Purpose:** ours\n",
		".agent-one/domain/http/README.md": "# domain-http\n\n- **Rank:** Domain owner\n- **Owns:** `api/`\n- **Reports to:** coord-team\n- **Purpose:** ours\n",
		".agent-one/zone/server/README.md": "# zone-server\n\n- **Rank:** Zone worker\n- **Owns:** `api/`\n- **Reports to:** domain-http\n- **Purpose:** ours\n",
	} {
		_ = os.MkdirAll(filepath.Dir(filepath.Join(root2, p)), 0o755)
		if err := os.WriteFile(filepath.Join(root2, p), []byte(agent), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	s2, res := sketchAt(t, root2)
	if !contains(res.covered, "zone-api") {
		t.Fatalf("api/ is already held: %+v", res)
	}
	for _, w := range res.wrote {
		if strings.Contains(w, "zone/api/") {
			t.Fatalf("the sketch wrote a second persistent agent for api/: %v", res.wrote)
		}
	}
	if w, err := onto.Load(root2); err != nil || len(w.Validate()) != 0 {
		t.Fatalf("the merged team checks clean: %v %v\n%s", err, w.Validate(), s2.table(lexiconFor().Prefixes))
	}
}

// The skills a workspace already has are held where they fit; none is invented.
func TestMediumHoldsFittingSkills(t *testing.T) {
	root := project(t, map[string]string{
		"go.mod":          "module example.com/m\n",
		"api/api.go":      goFile("api", "serves.", 300),
		"api/api_test.go": goTest("api"),
		".agent-one/base-skills/zone-go-proving/SKILL.md": "---\nname: zone-go-proving\ndescription: how this Go module proves itself\n---\n",
		".agent-one/base-skills/zone-py-proving/SKILL.md": "---\nname: zone-py-proving\ndescription: how the python half runs pytest\n---\n",
		".agent-one/base-skills/api-contract/SKILL.md":    "---\nname: api-contract\ndescription: the api's wire contract\n---\n",
	})
	s, _ := sketchAt(t, root)
	sl := byName(s)["zone-api"]
	if strings.Join(sl.Skills, ",") != "api-contract,zone-go-proving" {
		t.Errorf("skills held: %v", sl.Skills)
	}
	if doc := readDoc(t, root, "zone", "api"); !strings.Contains(doc, "- **Skills:** api-contract, zone-go-proving") {
		t.Errorf("the doc names its skills:\n%s", doc)
	}
}

// `init --level medium` on the wire: the census, the lanes, what was written and kept.
func TestInitMediumWire(t *testing.T) {
	root := project(t, map[string]string{
		"go.mod":                   "module example.com/w\n",
		"api/api.go":               goFile("api", "serves.", 300),
		"api/api_test.go":          goTest("api"),
		"store/store.go":           goFile("store", "stores.", 300, "example.com/w/api"),
		".github/workflows/ci.yml": "",
	})
	run := func() string {
		var out, errb bytes.Buffer
		env := func(k string) string {
			if k == "HOME" {
				return root
			}
			return ""
		}
		if code := Main("agent-one", []string{"--root", root, "init", "--level", "medium", "--plain"}, IO{In: strings.NewReader(""), Out: &out, Err: &errb, Env: env}, Version{}); code != 0 {
			t.Fatalf("init: %d %s %s", code, out.String(), errb.String())
		}
		return out.String()
	}
	out := run()
	for _, want := range []string{"@F .agent-one/zone/api/README.md", "@F lane coord-proj: domain-proj (2)", "(1 coordinator · 1 domain owner · 2 zone workers · 1 service owner): 5 members written (0 kept as they were)"} {
		if !strings.Contains(out, want) {
			t.Errorf("wire lacks %q:\n%s", want, out)
		}
	}
	if out := run(); !strings.Contains(out, "0 members written (5 kept as they were)") {
		t.Errorf("a second run keeps everything:\n%s", out)
	}
}

func TestNormLevel(t *testing.T) {
	for in, want := range map[string]string{"": "light", "soft": "light", "Light": "light", "medium": "medium", "complex": "complex", "huge": ""} {
		if got := normLevel(in); got != want {
			t.Errorf("normLevel(%q) = %q, want %q", in, got, want)
		}
	}
}

// The ontology's zone rule, mirrored: what a name may claim.
func TestZoneOK(t *testing.T) {
	for _, c := range []struct {
		short, entry string
		ok           bool
	}{
		{"app", "cmd/", true}, {"app", "main.go", true}, {"anything", "src/a/b/", true},
		{"kernel-loop", "kernel/loop/", true}, {"kernel", "kernel/loop/", true}, {"loop", "kernel/loop/", false},
		{"kernel-loop", "kernel/wire/", false}, {"packages-ui", "packages/ui/src/", true}, {"agent-one", "agent-one/engine/", false},
	} {
		if got := zoneOK(c.short, c.entry); got != c.ok {
			t.Errorf("zoneOK(%q, %q) = %v", c.short, c.entry, got)
		}
	}
}

// The complex level's brief carries the sketch as its first hint, in the distribution's words.
func TestFoundingBrief(t *testing.T) {
	root := tree(t, "go.mod", "svc/a/x.go", "svc/a/y.go", "svc/b/z.go")
	d, _ := parseDist("agent-one")
	lex := lexiconFor()
	s := sketchTeam(root, d)
	ask := foundingAsk(root, d, s, lex.Ranks, lex.Prefixes, lex.Display)
	if strings.Contains(ask, "{{") || !strings.Contains(ask, "zone-svc → domain-") || !strings.Contains(ask, "agent-one onto check") {
		t.Fatalf("the founding brief is not filled in:\n%s", ask)
	}
	a1 := lexiconFor()
	d1, _ := parseDist("agent-one")
	ask1 := foundingAsk(root, d1, sketchTeam(root, d1), a1.Ranks, a1.Prefixes, a1.Display)
	if !strings.Contains(ask1, a1.Prefixes["zone"]+"svc → "+a1.Prefixes["domain"]) || !strings.Contains(ask1, "agent-one onto check") || !strings.Contains(ask1, "**Rank:** Zone worker") {
		t.Fatalf("the brief speaks the distribution's words:\n%s", ask1)
	}
}
