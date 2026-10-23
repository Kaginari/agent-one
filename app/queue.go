package app

import (
	"fmt"
	"strings"

	"github.com/Kaginari/agent-one/queue"
)

// cmdQueue is `q`: the shared work queue, read and moved by the human. The human stands over
// every agent, so any held or claimed item is theirs to move; --as names an agent to act for.
func (a *App) cmdQueue(args []string, io IO) int {
	if a.Workspace == nil {
		fmt.Fprintf(io.Err, "@S FAIL\n@? no %s here\n", worldWord(a.Cfg.Dist.Name))
		return 2
	}
	s := queue.Open(a.Workspace.QueuePath(), func(string, string) bool { return true })
	by := strings.ToLower(a.Workspace.Lex.Human)
	if by == "" {
		by = "human"
	}
	return queue.CLI(s, by, args, a.Cfg.Output.Format == "json", io.Out, io.Err)
}
