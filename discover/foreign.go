package discover

import "strings"

// This is the one file that names the convention agent-one was forked from, and only to keep its
// machine-wide files out. A skill, command, agent or instruction file under ~/.claude or
// ~/.config/opencode that was written for that convention (it names it, its world dir or its
// law) belongs to that convention's sessions, not to this workspace's; agent-one does not wear,
// run or list it. The workspace's own files are never filtered. (The leak test exempts this file.)

var foreignMarkers = []string{"isekai"}

// Foreign says a machine-wide file belongs to the convention agent-one was forked from.
func Foreign(source, text string) bool {
	if !strings.HasSuffix(source, "-global") {
		return false
	}
	low := strings.ToLower(text)
	for _, m := range foreignMarkers {
		if strings.Contains(low, m) {
			return true
		}
	}
	return false
}
