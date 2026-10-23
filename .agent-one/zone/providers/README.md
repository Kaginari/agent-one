# zone-providers

- **Rank:** Zone worker
- **Owns:** `provider/`
- **Reports to:** domain-engine
- **Skills:** zone-go-package
- **Purpose:** the ground truth of the one standing outward act — a chat turn with a model: the shared request shape and the two vendor dialects plus the mock.

## Invariants
- provider.Retry (retry.go) is the one retry path, used by both clients: Retryable is 429, 500, 502, 503, 504 and 529; up to 3 retries, waiting Retry-After when it is 1–60 s, else Backoff (1 s, 2 s, 4 s — a package var tests shorten); never past the context; the last response is returned for the caller to read. A non-retryable status comes back at once.
- Model fallback is not here: app/providers.go wraps a primary and a secondary Provider and tries the secondary only after the primary's error (after its retries) names a transport fault, HTTP 404/429/5xx/529 or `finish_reason error` — never on a refusal. So error strings `anthropic: HTTP n: …` / `openai: HTTP n: …` and openai's errUpstream wording, ErrStalled and the `before message_stop` ending are a contract with that wrapper and with the retry ladder around it.
- anthropic: POST /v1/messages, anthropic-version 2023-06-01, DefaultModel claude-opus-5 (AGENT_ONE_MODEL overrides), max_tokens 16000 unless set; with Fallbacks on (New's default) it sends agent `fallbacks: "default"` and the anthropic-beta FallbackBeta header — the server-side refusal fallback. It declares Claude's trained tools schema-less (bash_20250124, text_editor_20250728), puts cache_control on the last stable system block, sends adaptive thinking and output_config.effort, and replays thinking blocks opaque on the next call.
- openai: ToolCalls "text" appends textToolIntro and parses <tool_call> tags (a bad tag becomes a call named `_malformed`); native arguments that are not JSON become `{"_malformed": …}`; Guided constrains a Wire request to WireSchema and RenderWire turns the JSON back into @-lines; ContextWindow reads GET /models once; finish_reason "error" under HTTP 200 is a failed call (errUpstream), not an answer; errorText reads OpenAI's and Gemini's error shapes. An empty APIKey is allowed (Ollama).
- Keys never cross the wire: provider.Getenv is a package var so tests inject an environment. Config-declared Headers are set verbatim on every request — config refuses authorization, x-api-key, api-key and secret-looking values there, so this package trusts what it is given.
- Provider.Name() is anthropic/<model> or openai/<model> (the fallback wrapper keeps the primary's); the usage journal and the board key spend by that name. Usage.Context() = Input + CacheRead + CacheWrite is what the instrument reads; Usage.Add sums a Subagent's spend into its parent.
- provider.Validate refuses before any send: no messages, two user messages in a row, a tool result that answers no call or a call left unanswered, an empty user message, a conversation not ending on the user.
- mock.Provider plays scripted Responses in order for tests and selftests; the app's bench uses its own scriptProvider (app/mockscript.go).
- watch.go: both clients wrap every response body in provider.Watch — the context's end (context.AfterFunc, no goroutine) or, on an SSE stream only and from its first byte on, StreamStall (2 min, a package var) with no byte closes it — time to first token stays the client timeout's, and the blocked read returns ctx.Err() or ErrStalled. A non-stream agent has no idle watch: it arrives whole after the generation.

## Verify
- `go test ./provider/...`

## Working notes
