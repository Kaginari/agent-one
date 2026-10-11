package config

import (
	"encoding/json"
	"strings"
	"testing"
)

// Every key of the defaults has a line saying what it does, and a switch says on/off.
func TestEveryDefaultKeyIsDescribed(t *testing.T) {
	cfg, err := LoadWith(Options{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(cfg)
	var tree any
	_ = json.Unmarshal(raw, &tree)
	docs := Docs(tree)
	var missing []string
	var walk func(v any, path string)
	walk = func(v any, path string) {
		if path != "" {
			d := docs[path]
			if d == "" {
				missing = append(missing, path)
			} else if strings.HasSuffix(path, ".enabled") && !strings.HasPrefix(d, "on/off") {
				t.Errorf("%s: a switch says on/off first: %q", path, d)
			}
		}
		if m, ok := v.(map[string]any); ok {
			for k, x := range m {
				if k[0] >= 'A' && k[0] <= 'Z' {
					continue
				}
				p := k
				if path != "" {
					p = path + "." + k
				}
				walk(x, p)
			}
		}
	}
	walk(tree, "")
	if len(missing) > 0 {
		t.Fatalf("%d keys without a line:\n%s", len(missing), strings.Join(missing, "\n"))
	}
	if d := Describe("providers.gateway.models.kimi.price.input"); !strings.Contains(d, "input") {
		t.Fatalf("a pattern resolves: %q", d)
	}
	if d := Describe("compaction.enabled"); !strings.HasPrefix(d, "on/off") {
		t.Fatalf("compaction.enabled: %q", d)
	}
}
