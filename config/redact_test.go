package config

import (
	"strings"
	"testing"
)

// RedactSecrets masks credentials by shape and by key, and leaves the rest of the text whole.
func TestRedactSecrets(t *testing.T) {
	for in, gone := range map[string]string{
		"OPENAI_API_KEY=sk-live0123456789abcdef":                                 "sk-live0123456789abcdef",
		`{"apiKey": "plainvalue123"}`:                                            "plainvalue123",
		"Authorization: Bearer abcdefghijklmnop":                                 "abcdefghijklmnop",
		"push with ghp_0123456789abcdefABCDEF0123456789abcd now":                 "ghp_0123456789abcdefABCDEF0123456789abcd",
		"aws AKIAABCDEFGHIJKLMNOP":                                               "AKIAABCDEFGHIJKLMNOP",
		"-d password=hunter2-correct-horse":                                      "hunter2-correct-horse",
		"-----BEGIN RSA PRIVATE KEY-----\nMIIabc\n-----END RSA PRIVATE KEY-----": "MIIabc",
	} {
		if got := RedactSecrets(in); strings.Contains(got, gone) || !strings.Contains(got, "[redacted]") {
			t.Errorf("%q → %q", in, got)
		}
	}
	for _, keep := range []string{"the build is green: 12 tests, tokens: 4096", "apiKeyEnv: OPENAI_API_KEY", "go test -count=1 ./workspace/", "see workspace/ownership.go:92"} {
		if got := RedactSecrets(keep); got != keep {
			t.Errorf("%q was changed: %q", keep, got)
		}
	}
}
