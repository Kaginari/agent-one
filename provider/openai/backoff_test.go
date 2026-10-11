package openai

import (
	"time"

	"github.com/Kaginari/agent-one/provider"
)

func init() { provider.Backoff = func(int) time.Duration { return time.Millisecond } }
