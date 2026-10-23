package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// StreamStall is how long a streamed body may go without a byte before it is called dead; tests
// shorten it. Server-sent events carry pings, so a live stream never falls this silent — the
// shape it catches is a gateway that holds the socket open and sends nothing, which would
// otherwise sit on the client's whole timeout.
var StreamStall = 2 * time.Minute

// ErrStalled is a stream that fell silent past its stall window.
var ErrStalled = errors.New("stream stalled")

// Watch guards a response body: the context's end, or a gap of idle with no byte (0 = no idle
// watch), closes it — a read blocked on a silent socket returns at once, and with the reason
// (ctx.Err() or ErrStalled) rather than the transport's "use of closed connection". No goroutine
// is held: the context's AfterFunc and one timer do the watching.
func Watch(ctx context.Context, body io.ReadCloser, idle time.Duration) io.ReadCloser {
	w := &watched{body: body, idle: idle}
	w.unhook = context.AfterFunc(ctx, func() { w.kill(ctx.Err()) })
	if idle > 0 {
		w.timer = time.AfterFunc(idle, func() { w.kill(fmt.Errorf("%w: no byte for %s", ErrStalled, idle)) })
	}
	return w
}

type watched struct {
	body   io.ReadCloser
	idle   time.Duration
	timer  *time.Timer
	unhook func() bool
	mu     sync.Mutex
	why    error // set once, by the first watcher that fires
}

func (w *watched) kill(why error) {
	w.mu.Lock()
	if w.why == nil {
		w.why = why
	}
	w.mu.Unlock()
	_ = w.body.Close()
}

func (w *watched) cause() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.why
}

func (w *watched) Read(p []byte) (int, error) {
	n, err := w.body.Read(p)
	if n > 0 && w.timer != nil {
		w.timer.Reset(w.idle)
	}
	if err != nil && err != io.EOF {
		if why := w.cause(); why != nil {
			err = why
		}
	}
	return n, err
}

func (w *watched) Close() error {
	w.unhook()
	if w.timer != nil {
		w.timer.Stop()
	}
	return w.body.Close()
}
