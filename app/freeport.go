package app

import (
	"math/rand"
	"net"
	"strconv"
	"time"
)

// boardReservedPorts are the estate/machine ports init never offers to a new board: the local
// the machine.local LLM proxy (4000), this binary's own current board default (7411), the conduit gateway pair
// (8080/8081) and the fake-ai bench gateway (8090); 8091 stays clear as its known sibling.
var boardReservedPorts = map[int]bool{4000: true, 7411: true, 8080: true, 8081: true, 8090: true, 8091: true}

// boardDefaultPort mirrors config/defaults.go's ui.board.port.
const boardDefaultPort = 7411

// pickBoardPort answers a free localhost port for the board at init: random picks from the
// 8092-8199 band, each actually tried with a Listen so an occupied port never lands in a written
// config. When the whole band is somehow busy, the OS assigns one.
func pickBoardPort() int {
	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	band := make([]int, 0, 8199-8092+1)
	for p := 8092; p <= 8199; p++ {
		if !boardReservedPorts[p] {
			band = append(band, p)
		}
	}
	for len(band) > 0 {
		i := r.Intn(len(band))
		p := band[i]
		band = append(band[:i], band[i+1:]...)
		if l, err := net.Listen("tcp", "127.0.0.1:"+strconv.Itoa(p)); err == nil {
			_ = l.Close()
			return p
		}
	}
	if l, err := net.Listen("tcp", "127.0.0.1:0"); err == nil {
		defer func() { _ = l.Close() }()
		return l.Addr().(*net.TCPAddr).Port
	}
	return boardDefaultPort // a busy shell finds out at the next session's board start
}
