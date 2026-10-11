package board

import (
	"os"
	"path/filepath"
	"testing"
)

func TestRelatesCoupleImportsAndSharedSkills(t *testing.T) {
	root := t.TempDir()
	write := func(rel, agent string) {
		p := filepath.Join(root, rel)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(agent), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("go.mod", "module example.com/w\n\ngo 1.22\n")
	write("store/db.go", "package store\n\nimport (\n\t\"fmt\"\n\t\"example.com/w/entity\"\n)\n\nvar _ = fmt.Sprint\nvar _ = entity.X\n")
	write("store/more.go", "package store\n\nimport \"example.com/w/entity\"\n\nvar _ = entity.X\n")
	write("store/db_test.go", "package store\n\nimport \"example.com/w/web\"\n\nvar _ = web.Y\n")
	write("entity/e.go", "package entity\n\nvar X = 1\n")
	write("web/ui/app.js", "import { y } from '../lib/util.js';\nconst z = require('./local');\n")
	write("web/lib/util.js", "export const y = 1;\n")
	write("cli/main.py", "from tools import helper\nimport os\n")
	write("tools/helper.py", "x = 1\n")
	nodes := []Node{
		{ID: "orchestrator", Kind: "member", Rank: "orchestrator", Orchestrator: true},
		{ID: "domain-data", Kind: "member", Rank: "domain", Parent: "orchestrator", Owns: []string{"store", "entity"}},
		{ID: "zone-db", Kind: "member", Rank: "zone", Parent: "domain-data", Owns: []string{"store"}},
		{ID: "zone-entities", Kind: "member", Rank: "zone", Parent: "domain-data", Owns: []string{"Entity"}},
		{ID: "zone-ui", Kind: "member", Rank: "zone", Parent: "orchestrator", Owns: []string{"web/ui"}},
		{ID: "zone-lib", Kind: "member", Rank: "zone", Parent: "orchestrator", Owns: []string{"web/lib/*.js"}},
		{ID: "zone-cli", Kind: "member", Rank: "zone", Parent: "orchestrator", Owns: []string{"cli"}},
		{ID: "zone-tools", Kind: "member", Rank: "zone", Parent: "orchestrator", Owns: []string{"tools"}},
		{ID: "skill-core", Kind: "skill"},
	}
	edges := []Edge{
		{From: "zone-ui", To: "skill-core", Bond: "holds"},
		{From: "zone-db", To: "skill-core", Bond: "holds"},
		{From: "domain-data", To: "skill-core", Bond: "holds"}, // the zone worker's ancestor: no relate
	}
	got := map[string]Relate{}
	for _, r := range Relates(root, nodes, edges) {
		got[r.From+"~"+r.To] = r
	}
	if r := got["zone-db~zone-entities"]; r.Imports != 2 || r.Weight != 1 {
		t.Errorf("db ↔ entities: want 2 imports at weight 1 (the strongest), got %+v", r)
	}
	if r := got["zone-lib~zone-ui"]; r.Imports != 1 {
		t.Errorf("a relative JS import through a glob-owned path: want 1 import, got %+v", r)
	}
	if r := got["zone-cli~zone-tools"]; r.Imports != 1 {
		t.Errorf("a Python import: want 1, got %+v", r)
	}
	if r := got["zone-db~zone-ui"]; r.Skills != 1 || r.Imports != 0 {
		t.Errorf("a shared skill (the test file's import left out): want 1 skill and no import, got %+v", r)
	}
	for k := range got {
		if k == "domain-data~zone-db" || k == "domain-data~zone-entities" {
			t.Errorf("an ancestor pair is the tree's business, not a relate: %s", k)
		}
	}
	if rs := Relates(t.TempDir(), nil, nil); rs == nil || len(rs) != 0 {
		t.Errorf("an empty workspace: want an empty, non-nil list, got %#v", rs)
	}
}
