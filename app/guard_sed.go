package app

import (
	"encoding/json"
	"regexp"
	"strings"

	"github.com/Kaginari/agent-one/loop"
)

// reSedInPlace catches in-place sed edits inside a shell command segment:
//
//	sed -i 's/old/new/' file.go          (GNU)
//	sed -i '' 's/old/new/' file.go       (macOS sudo-less form)
//	sed -i.bak -e 's/old/new/' file.go
//	sed --in-place 's/old/new/' file.go
//
// Streams and reads stay free: `sed -n '100,260p'`, `sed 's/x/y/' < in > out`,
// `printf | sed …`. Only the anchored silent class is refused — an in-place sed whose anchor
// misses rewrites nothing and still exits 0: the agent's intent dies unrecorded (no anchor
// verification, no loud failure, no res.Wrote), and the verification re-reads that follow were a
// live doom-feeder on 2026-10-03 (bench echoes at floor). The edit tool carries the anchor
// honestly: no anchor → loud fail; drift → receipt; write → attributed.
var reSedInPlace = regexp.MustCompile(`(^|[;&|]\s*)\bsed\b[^;&|]*?(?:\s-=?i(\.\S+|\s+\S+)?|--in-place(=\S+)?)(\s|$)`)

// sedGuardHook refuses anchored in-place sed edits through bash. The rule is absolute (Operator
// 2026-10-03: "always use edit") — the refusal names both what died (the anchor) and what to
// reach for; an agent needing a bulk mechanical transform can decompose it into edits, or the
// human can run the sed itself.
func (a *App) sedGuardHook() func(s *loop.Session, tool string, input json.RawMessage) string {
	if !a.Cfg.Tools.TmpRedirect.Enabled { // the workspace's write-discipline flag covers the family
		return nil
	}
	return func(s *loop.Session, name string, input json.RawMessage) string {
		if name != "bash" {
			return ""
		}
		var in struct {
			Command string `json:"command"`
		}
		if json.Unmarshal(input, &in) != nil {
			return ""
		}
		m := reSedInPlace.FindStringSubmatch(in.Command)
		if m == nil {
			return ""
		}
		frag := strings.TrimSpace(m[0])
		return "refused: `" + frag + "` is an in-place sed edit — use the `edit` tool instead: it verifies the anchor, " +
			"attributes the write, and fails loudly when the anchor is absent (an anchored sed silently no-ops and " +
			"the verification re-reads after it feed the guts' floor). sed stays free for streams: `sed -n N,Mp` reads, " +
			"`sed s/.../.../ < in > out` shaping in pipes. Bulk mechanical transforms: decompose into edits."
	}
}
