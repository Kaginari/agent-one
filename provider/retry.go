package provider

import (
	"context"
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
