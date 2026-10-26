package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Retryable is an HTTP status worth another try: a rate limit, an overload (529 is Anthropic's),
// or a gateway fault.
func Retryable(code int) bool {
	return code == 429 || code == 500 || code == 502 || code == 503 || code == 504 || code == 529
}

// StreamError is a failure the server reports inside a stream it already opened under HTTP 200:
// OpenRouter relays an upstream provider's failure this way, after the headers and often after
// some text. Code is the status the event names, 0 when it names none.
type StreamError struct {
	Provider, Type, Message string
	Code                    int
}

func (e *StreamError) Error() string {
	kind := e.Type
	if e.Code != 0 {
		kind = strings.TrimSpace(fmt.Sprintf("%s %d", e.Type, e.Code))
	}
	return fmt.Sprintf("%s: stream error %s: %s", e.Provider, kind, e.Message)
}

// Transient says whether asking again may succeed. The request was accepted — the stream began —
// so a failure here is the server's or its upstream's unless the event says the request was at
// fault: a 4xx other than 429, or a type naming an invalid, unauthorised or unknown request.
func (e *StreamError) Transient() bool {
	if e.Code != 0 {
		return Retryable(e.Code) || e.Code >= 500
	}
	t := strings.ToLower(e.Type)
	for _, w := range []string{"invalid", "auth", "permission", "not_found", "context_length"} {
		if strings.Contains(t, w) {
			return false
		}
	}
	return true
}

// Backoff is the wait before retry n (0-based) when the server names none; tests shorten it.
var Backoff = func(attempt int) time.Duration { return time.Duration(1<<attempt) * time.Second }

// Retry sends a request, retrying transient failures — Retry-After when the server gives one
// (up to a minute), else Backoff — up to 3 retries, and never past the context. send builds a
// fresh request each time. The last response is returned as it is for the caller to read.
func Retry(ctx context.Context, send func() (*http.Response, error)) (*http.Response, error) {
	const retries = 3
	for attempt := 0; ; attempt++ {
		res, err := send()
		if err == nil && !Retryable(res.StatusCode) {
			return res, nil
		}
		if attempt == retries || ctx.Err() != nil {
			return res, err
		}
		wait := Backoff(attempt)
		if err == nil {
			if s, perr := strconv.Atoi(strings.TrimSpace(res.Header.Get("Retry-After"))); perr == nil && s > 0 && s <= 60 {
				wait = time.Duration(s) * time.Second
			}
			_, _ = io.Copy(io.Discard, io.LimitReader(res.Body, 1<<20))
			res.Body.Close()
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(wait):
		}
	}
}
