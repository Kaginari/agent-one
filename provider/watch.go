package provider

import (
	"context"
	"errors"
	"fmt"
	"io"
	"sync"
	"time"
)

// StreamStall is how long a streamed body may fall silent once it has begun before it is called
// dead; tests shorten it. The window opens on the first byte: a queue wait or a long prefill
// (vLLM, Ollama and llama.cpp send nothing until the first token) stays the client timeout's to
// bound. What it catches is a stream that started and stopped — a gateway holding the socket
// open with nothing more to say, which would otherwise sit on the client's whole timeout.
var StreamStall = 2 * time.Minute

// ErrStalled is a stream that fell silent past its stall window.
var ErrStalled = errors.New("stream stalled")

// Watch guards a response body: the context's end, or — once the first byte has come — a gap of
// idle with no byte (0 = no idle watch), closes it; a read blocked on a silent socket returns at
// once, and with the reason (ctx.Err() or ErrStalled) rather than the transport's "use of closed
// connection". No goroutine is held: the context's AfterFunc and one timer do the watching.
func Watch(ctx context.Context, body io.ReadCloser, idle time.Duration) io.ReadCloser {
	w := &watched{body: body, idle: idle}
	w.unhook = context.AfterFunc(ctx, func() { w.kill(ctx.Err()) })
	return w
}

type watched struct {
	body   io.ReadCloser
	idle   time.Duration
	unhook func() bool
	mu     sync.Mutex
	timer  *time.Timer // armed by the first byte
	closed bool
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
	if n > 0 && w.idle > 0 {
		w.mu.Lock()
		switch {
		case w.closed:
		case w.timer == nil:
			w.timer = time.AfterFunc(w.idle, func() { w.kill(fmt.Errorf("%w: no byte for %s", ErrStalled, w.idle)) })
		default:
			w.timer.Reset(w.idle)
		}
		w.mu.Unlock()
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
	w.mu.Lock()
	w.closed = true
	if w.timer != nil {
		w.timer.Stop()
	}
	w.mu.Unlock()
	return w.body.Close()
}
