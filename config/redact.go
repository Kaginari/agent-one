package config

import (
	"regexp"
	"strings"
)

// RedactSecrets masks what looks like a credential in free text bound for somewhere outward (a
// trace collector): a PEM private key, an Authorization scheme's credential, a token whose shape
// names it (looksSecret's sk- / sk_, a GitHub, Slack, AWS or Google key, a JWT), and the value of
// any key=value / key: value whose key names a secret. Everything else is left as it is.
func RedactSecrets(s string) string {
	s = pemKey.ReplaceAllString(s, redacted)
	s = authScheme.ReplaceAllString(s, "${1} "+redacted)
	s = tokenWord.ReplaceAllStringFunc(s, func(w string) string {
		if looksSecret(w) || knownToken.MatchString(w) {
			return redacted
		}
		return w
	})
	return secretPair.ReplaceAllStringFunc(s, func(m string) string {
		p := secretPair.FindStringSubmatch(m)
		if !secretName(p[1]) || p[3] == redacted {
			return m
		}
		return p[1] + p[2] + redacted
	})
}

const redacted = "[redacted]"

var (
	pemKey     = regexp.MustCompile(`-----BEGIN [A-Z ]*PRIVATE KEY-----[\s\S]*?-----END [A-Z ]*PRIVATE KEY-----`)
	authScheme = regexp.MustCompile(`(?i)\b(bearer|basic|token)\s+[A-Za-z0-9._~+/=-]{8,}`)
	tokenWord  = regexp.MustCompile(`[A-Za-z0-9][A-Za-z0-9._\-]{11,}`)
	knownToken = regexp.MustCompile(`^(gh[pousr]_[A-Za-z0-9]{20,}|github_pat_[A-Za-z0-9_]{20,}|xox[abprs]-[A-Za-z0-9-]{10,}|AKIA[0-9A-Z]{16}|AIza[0-9A-Za-z_\-]{30,}|eyJ[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]+\.[A-Za-z0-9_\-]*)$`)
	// key, the separator (quotes, = or :, spaces), the value up to a quote, space or delimiter
	secretPair = regexp.MustCompile(`([A-Za-z0-9_.\-]+)(["']?\s*[:=]\s*["']?)([^\s"'&,;]{4,})`)
)

// secretName says a key names a credential: an api key, password, secret, token, credential or
// auth — never a count of tokens or the NAME of an env var holding one (…Env).
func secretName(k string) bool {
	l := strings.ToLower(k)
	if strings.HasSuffix(l, "env") || strings.Contains(l, "tokens") {
		return false
	}
	for _, s := range []string{"apikey", "api_key", "api-key", "password", "passwd", "secret", "token", "credential", "auth"} {
		if strings.Contains(l, s) {
			return true
		}
	}
	return false
}
