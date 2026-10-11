package workspace

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/Kaginari/agent-one/wire"
)

func TestZonePolicyPairingGatePassthrough(t *testing.T) {
	dir := fixture(t)
	w := open(t, dir)

	// replicate the live workspace: zone-policy claims .agent-one/config.local.yaml as ownership
	// (per b413fdc) and the README is inside its own dir. Mutate the member in place.
	docPath := ".agent-one/zone/policy/README.md"
	os.WriteFile(filepath.Join(dir, docPath), []byte("# zone-policy\n- **Rank:** Zone worker\n- **Owns:** `.agent-one/config.local.yaml`\n- **Reports to:** domain-core\n"), 0o644)
	os.MkdirAll(filepath.Join(dir, ".agent-one/zone/policy"), 0o755)
	w.Reload()

	fmt.Printf("=== roster after policy member added ===\n")
	for _, c := range w.Members {
		fmt.Printf("  %s dir=%q terr=%v\n", c.Name, c.Dir, c.Ownership)
	}

	// what orchestrator's turn wrote: config.local.yaml AND the README. The gate fired on both.
	owner1 := w.Owner(".agent-one/config.local.yaml")
	owner2 := w.Owner(docPath)
	fmt.Printf("owner(config.local.yaml)=%v  owner(README)=%v\n", nameOf(owner1), nameOf(owner2))

	opt := DefaultHooks().Gate
	opt.InvariantsHold = false
	opt.DocTruthful = false
	v := w.Gate(context.Background(), nil, opt, "orchestrator", []string{".agent-one/config.local.yaml", docPath}, &wire.Report{Status: "DONE"}, false, "founding")
	fmt.Printf("verdict: %q reasons=%v\n", v.Word, v.Reasons)
}

func nameOf(c *Member) string {
	if c == nil {
		return "<nil>"
	}
	return c.Name
}
