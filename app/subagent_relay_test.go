package app

import (
	"strings"
	"testing"

	"github.com/Kaginari/agent-one/wire"
)

// relay bookkeeping: expect the commission's @$ echo at Finish, one wake per unbooked order.
func TestSubagentRelayBookkeeping(t *testing.T) {
	c := newSubagent(testTmpApp(t, t.TempDir()))
	c.ExpectRelay("coord-agent-one", wire.EnqueuesOf("…work the findings…\n@$ zone-tui-board: land the Arm A fixtures per the fence\n@E 0"))
	c.Track("coord-agent-one", "coord", "analyst", "kimi", nil, 1)
	if len(c.relays["coord-agent-one"]) != 1 {
		t.Fatal("expectation not registered: ", c.relays)
	}
	// truthful echo: report carries the @$ → no wake
	c.Finish("coord-agent-one", "@S DONE\n@$ zone-tui-board: land the Arm A fixtures per the fence\n@E 0")
	if woke := c.TakeWake(); len(woke) != 0 {
		t.Fatalf("echoed relays wake nobody: %v", woke)
	}
	// no echo, agent never tracked → one unbooked wake naming agent + ask
	c.ExpectRelay("coord-agent-one", wire.EnqueuesOf("@$ ring-wraith: cross the ford with the findings\n@E 0"))
	c.Finish("coord-agent-one", "@S DONE\n@F strong\n@E 0")
	woke := c.TakeWake()
	if len(woke) == 0 || !strings.Contains(woke[0], "[relay unbooked]") || !strings.Contains(woke[0], "ring-wraith") || !strings.Contains(woke[0], "cross the ford") {
		t.Fatalf("relay unbooked must be named with agent and ask: %v", woke)
	}
	// booking by tracking: the agent sits on the board running → no wake
	c.ExpectRelay("coord-agent-one", wire.EnqueuesOf("@$ zone-memory: chair the file kit beacon\n@E 0"))
	c.Track("zone-memory", "zone", "analyst", "kimi", nil, 2)
	if len(c.relays["coord-agent-one"]) != 0 {
		t.Error("tracking clears the expectation")
	}
	c.Finish("coord-agent-one", "@S DONE\n@E 0")
	if woke := c.TakeWake(); len(woke) != 0 {
		t.Fatalf("tracked relays booked: %v", woke)
	}
}
