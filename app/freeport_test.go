package app

import (
	"net"
	"strconv"
	"testing"
)

func TestPickBoardPortFree(t *testing.T) {
	p := pickBoardPort()
	if boardReservedPorts[p] {
		t.Fatalf("picked a reserved port: %d", p)
	}
	if p < 8092 || p > 8199 {
		// in-band expected when the band has free ports; an OS-assigned fallback is legal but
		// unlikely on a dev machine — fail only if the band was actually free:
		l, err := net.Listen("tcp", "127.0.0.1:8092")
		if err == nil {
			_ = l.Close()
			t.Fatalf("band free but port outside 8092-8199: %d", p)
		}
	}
	l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p))
	if err != nil {
		t.Fatalf("picked port %d, but cannot listen on it: %v", p, err)
	}
	_ = l.Close()
}
