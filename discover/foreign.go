package discover

import (
	"regexp"
	"strings"
)

// This is the one file that names the convention agent-one was forked from, and only to keep its
// machine-wide files out. A skill, command, agent or instruction file under ~/.claude or
// ~/.config/opencode that was written for that convention (it names it, its world dir or its
// law) belongs to that convention's sessions, not to this workspace's; agent-one does not wear,
// run or list it. The workspace's own files are never filtered. (The leak test exempts this file.)

// its own words: the convention, its session and its human, its ranks, its rank-prefixed names
var foreignMarker = regexp.MustCompile(`(?i)\b(isekai|rimuru|veldora|slimes?|kijin)\b|\b(orc|elf|slime|kijin)-[a-z]`)

// Foreign says a machine-wide file belongs to the convention agent-one was forked from.
func Foreign(source, text string) bool {
	if !strings.HasSuffix(source, "-global") {
		return false
	}
	return foreignMarker.MatchString(text)
}
