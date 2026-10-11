package tool

import (
	"context"
	"encoding/json"
)

// PulseTool is `pulse`: the acting agent's own run state — steps, turns, wall, context,
// momentum, the benches it has struck (visits with red/green/muted counts) and its writes.
// The loop-state blindspot cure (2026-10-09 audit: a zone worker hit one bench 63 times, every
// health guard silent): an agent that can read its own momentum and bench tally can break
// its own check-loop without a subagent. Class read; the state is served by the loop through
// Env.Self — nil outside a session (the tool says so, never errors).
func PulseTool() *Tool {
	return &Tool{
		Name:        "pulse",
		Description: "Your own live run state: steps, turns, wall clock, context zone, momentum, the benches you have hit (visits with red/green/muted counts) and your writes. Read it before re-running a bench you have already hit several times — repeated reds with no green is a check-loop, not TDD; change the approach or answer on the wire.",
		Schema:      json.RawMessage(`{"type":"object","properties":{},"additionalProperties":false}`),
		Class:       Read,
		Run: func(ctx context.Context, env Env, in json.RawMessage) Result {
			if env.Self == nil {
				return Result{Output: "(self-state is not served in this context — no session is acting)"}
			}
			return Result{Output: env.Self()}
		},
	}
}
