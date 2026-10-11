package app

import (
	"strings"
	"testing"
)

// A path inside the workspace is never herded into scratch, wherever the workspace lives — /app in a
// benchmark's container, a dir under /tmp or /var; what leaves the workspace still is.
func TestTmpRedirectLeavesTheWorkspaceItsOwnPaths(t *testing.T) {
	scratch := "/app/.agent-one/tmp/scratch/r1"
	for _, tc := range []struct{ cmd, workspace string }{
		{"printf '%s\\n' 'hello bench' > /app/hello.txt && cat /app/hello.txt", "/app"},
		{"cat /tmp/w/notes.md", "/tmp/w"},
		{"ls /var/lib/proj/src", "/var/lib/proj"},
	} {
		if out, msg := rewriteTmpCommand(tc.cmd, scratch, tc.workspace, false); out != tc.cmd || msg != "" {
			t.Errorf("%q in workspace %s was rewritten: %q %q", tc.cmd, tc.workspace, out, msg)
		}
	}
	out, _ := rewriteTmpCommand("echo x > /tmp/out.txt && cat /app/hello.txt", scratch, "/app", false)
	if !strings.Contains(out, scratch+"/out.txt") || !strings.Contains(out, "/app/hello.txt") || strings.Contains(out, "root/app") {
		t.Fatalf("off-workspace /tmp is herded, the workspace's own path is kept: %q", out)
	}
}
