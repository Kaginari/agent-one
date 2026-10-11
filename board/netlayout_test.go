package board

import (
	"os/exec"
	"path/filepath"
	"testing"
)

// TestNetLayoutHolds runs the 2D net's layout check (testdata/netlayout/check.mjs): a ~60-member
// fixture laid out at the boxes the dashboard gives the net at 1280, 1440 and 1920 wide — every
// tile inside the view, no label cut below six ems. It needs node; without it the check is skipped
// (run it by hand: node board/testdata/netlayout/check.mjs).
func TestNetLayoutHolds(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("node is not on the PATH — the net's layout check runs in node")
	}
	out, err := exec.Command(node, filepath.Join("testdata", "netlayout", "check.mjs")).CombinedOutput()
	if err != nil {
		t.Fatalf("the net's layout broke a promise: %v\n%s", err, out)
	}
	t.Logf("\n%s", out)
}
