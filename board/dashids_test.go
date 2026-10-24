package board

import (
	"regexp"
	"strings"
	"testing"
)

// Every element dash.js reaches by id is on the page: a renamed id in one file and not the other
// throws inside a poll, and the page reads "offline" with the session up.
func TestDashScriptFindsEveryIDOnThePage(t *testing.T) {
	js, err := content.ReadFile("web/dash.js")
	if err != nil {
		t.Fatal(err)
	}
	page, err := content.ReadFile("web/dash.html")
	if err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, m := range regexp.MustCompile(`\$\('#([A-Za-z0-9_-]+)'\)`).FindAllStringSubmatch(string(js), -1) {
		id := m[1]
		if seen[id] {
			continue
		}
		seen[id] = true
		if !strings.Contains(string(page), `id="`+id+`"`) {
			t.Errorf("dash.js reaches #%s, which dash.html does not carry", id)
		}
	}
	if len(seen) < 20 {
		t.Fatalf("read %d ids from dash.js: the pattern no longer matches the script", len(seen))
	}
}
