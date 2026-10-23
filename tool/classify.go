package tool

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Kaginari/agent-one/guard"
)

// A port of loop.js's classifier: a heuristic, not a proof — it errs toward asking. A command is
// split at pipes and separators; the strongest class of any segment wins. Patterns match at a
// command position (line start, after ; & | ( ` $ or a quote) so `bash -c "git push"` is still
// outward. Unknown commands default to write; declare a class to tighten.

// systemPrefixes are readable from inside the workspace; a write there leaves it (writesSystem).
// /etc is not among them: it holds the machine's configuration and secrets (passwd, shadow,
// keys), so even a read of it leaves the workspace and asks.
var systemPrefixes = []string{"/dev/", "/usr/", "/bin/", "/sbin/", "/lib/", "/lib32/", "/lib64/", "/proc/", "/sys/", "/opt/", "/snap/"}

// harmlessDevices take a write without anything leaving the workspace.
var harmlessDevices = []string{"/dev/null", "/dev/stdout", "/dev/stderr", "/dev/tty", "/dev/fd/"}

// writesSystem reports whether a write to abs lands in a system directory.
func writesSystem(abs string) bool {
	for _, d := range harmlessDevices {
		if abs == d || strings.HasSuffix(d, "/") && strings.HasPrefix(abs, d) {
			return false
		}
	}
	for _, s := range append(systemPrefixes, "/etc/") {
		if strings.HasPrefix(abs, s) {
			return true
		}
	}
	return false
}

func at(s string) *regexp.Regexp { return regexp.MustCompile("(?:^|[\\s;&|(`$'\"])" + s) }

const record = `(AGENT-ONE\.md|log\.md|design/[^\s]*\.md|notes\.jsonl)`

// git's global options may sit between `git` and the verb (`git -C . push`, `git -c k=v push`,
// `git --no-pager push`); the verb still decides the class.
const gitOpt = `(?:-[cC]\s+\S+|-[cC]\S+|--(?:git-dir|work-tree|namespace|exec-path|super-prefix|config-env)\s+\S+|--?[A-Za-z][\w-]*(?:=\S*)?)`
const git = `git(?:\s+` + gitOpt + `)*\s+`

type rule struct {
	re  *regexp.Regexp
	why string
}

var destructiveRules = []rule{
	{at(`rm(\s|$)`), "rm"}, {at(`(shred|wipe|srm)(\s|$)`), "shred"}, {at(`(mv|rename)\s`), "mv overwrites"}, {at(`(truncate|dd|mkfs(\.\w+)?)(\s|$)`), "truncate/dd/mkfs"}, {at(`diskutil\s+(erase\w*|partitionDisk|zeroDisk|randomDisk|secureErase|reformat)`), "disk erase"},
	{at(git + `(reset|clean|filter-branch|filter-repo|rebase|gc|prune|rm|mv)\b`), "git history/tree rewrite"}, {at(git + `branch\b[^|;&]*\s-[dDM]\b`), "git branch delete/rename"},
	{at(git + `push\b[^|;&]*(\s--force\b|\s-f\b|\s\+)`), "git push --force"}, {at(git + `(checkout|restore)\s+(--\s|\.(\s|$))`), "git discard of working changes"},
	{at(git + `stash\s+(drop|clear|pop|push|save|apply)`), "git stash hides the tree under judgment"},
	{ // a bare stash and a flag-first one (`stash -u`) both mean push: the tree goes quiet same way
		at(git + `stash\b(\s+-\S+)*\s*($|[;&|])`), "git stash hides the tree under judgment",
	},
	{at(git + `reflog\s+(expire|delete)`), "git reflog purge"},
	{regexp.MustCompile(`:\(\)\s*\{`), "fork bomb"}, {regexp.MustCompile(`>\s*/dev/(r?disk|sd|nvme|hd|mmcblk|xvd|vd)`), "write to a raw disk"}, {at(git + `tag\s+-d\b`), "git tag delete"}, {regexp.MustCompile(`\s-delete(\s|$)`), "find -delete"},
	{regexp.MustCompile(`(^|[^>])>\s*\S*` + record), "overwrite of a record (> path)"}, {regexp.MustCompile(`sed\s+(-\S*i|--in-place)[^|;&]*` + record), "sed -i on a record"},
	{regexp.MustCompile(`tee\s+(-[^a\s]\S*\s+)*[^-|;&][^|;&]*` + record), "tee over a record"}, {regexp.MustCompile(`(cp|install)\s+[^|;&]*` + record + `\s*($|[;&|])`), "cp over a record"},
}

var outwardRules = []rule{
	{at(git + `(push|fetch|pull|clone|ls-remote|submodule\s+(update|add))\b`), "git ↔ remote"}, {at(git + `remote\s+(add|set-url|remove|rm|prune|update)`), "git remote change"},
	{at(`(curl|wget|ssh|scp|sftp|rsync|nc|ncat|netcat|telnet|ping|dig|nslookup|ftp|socat)\s`), "network"}, {at(`(npm|pnpm|yarn)\s+(publish|install|i|add|ci|update|upgrade|exec|x|link)\b`), "package network"},
	{at(`npx\s`), "npx fetches"}, {at(`pip3?\s+(install|download)`), "pip network"}, {at(`(docker|podman)\s+(push|pull|login|build|run)\b`), "container registry"},
	{at(`(gh|glab|hub|aws|gcloud|az|kubectl|helm|terraform|heroku|flyctl|vercel|netlify|firebase)\s`), "external service CLI"}, {at(`(mail|sendmail|mutt|msmtp)\s`), "mail"},
	{regexp.MustCompile(`https?://`), "URL"}, {at(`(xdg-open|open)\s`), "opens outside"},
	{at(`go\s+(get|install|mod\s+(download|tidy))\b`), "go module network"},
	// a secret store's secrets would cross the workspace's border: anchored at the segment's start, so
	// the word in prose (`echo "pass the token"`) is not a secret store
	{regexp.MustCompile(`^(pass|gopass|op|bw|bws|lpass|keepassxc-cli|rbw|nordpass)(\s|$)`), "secret store"},
	{regexp.MustCompile(`^security\s+(-\S+\s+)*(dump-keychain|find-generic-password|find-internet-password|export)`), "keychain"},
	{regexp.MustCompile(`^gpg\s[^|;&]*--export-secret`), "secret key export"}, {at(`brew\s+(install|uninstall|reinstall|upgrade|tap)\b`), "system packages"},
}

// vocabularyWriteRules: the same "writes files" verbs when they take the vocabulary's words as
// names are outward at the floor — the corpus asks on `chown -R someuser /`, never on
// `chmod 777 ./script.sh`; `Makisu/auth` is this workspace's own word for the pair.
var vocabularyWriteRules = []rule{
	{at(`(tee|cp|mkdir|touch|ln|chmod|chown|chgrp|install|patch|unzip|tar)\s[^|;&]*(/etc/|/usr/|/bin/|/sbin/|/var/|/boot/|/lib/|/opt/|/root|/System|/Library|chmod|chown|chgrp)`), "vocabulary rewrite on system paths"},
	{at(`(chmod|chown|chgrp)\s[^|;&]*\s/\s*$`), "vocabulary rewrite on system paths"},
}

var writeRules = []rule{
	{regexp.MustCompile(`(^|[^>])>>?`), "redirect"}, {at(`(tee|cp|mkdir|touch|ln|chmod|chown|chgrp|install|patch|unzip|tar)\s`), "writes files"}, {at(`sed\s+(-\S*i|--in-place)`), "sed -i"},
	{at(git + `(add|commit|checkout|switch|merge|stash|tag|init|apply|cherry-pick|notes|worktree)\b`), "git local write"}, {at(`memory\.js\s+[^|;&]*\b(remember|index|forget)\b`), "memory write"},
	// a formatter reads unless told to write: gofmt -l/-d is a look, -w rewrites; go fmt always does
	{at(`(gofmt|goimports)(\s[^|;&]*)?\s--?w(=\S*)?(\s|$)`), "formatter write"}, {at(`go\s+fmt\b`), "formatter write"},
}

var readOnly = regexp.MustCompile(`^(cat|ls|head|tail|wc|grep|egrep|fgrep|rg|ag|find|stat|file|which|type|echo|printf|test|\[|\[\[|true|false|pwd|date|env|printenv|sleep|sort|uniq|cut|tr|diff|cmp|md5sum|sha\d*sum|jq|yq|awk|sed|less|more|basename|dirname|realpath|readlink|du|df|tree|column|nl|tac|rev|od|xxd|hexdump|strings|seq|expr|bc|comm|paste|join|fold|fmt|xargs|` + git + `(status|log|diff|show|rev-parse|ls-files|blame|describe|cat-file|branch(\s+(-a|-r|-v|-vv|--list|-l))*\s*$|remote(\s+-v)?\s*$|shortlog|grep|count-objects|rev-list)|gofmt|goimports|node\s+\S*memory\.js\s+[^|;&]*\b(recall|status)\b|go\s+(version|env|list|vet|test|build|doc)\b)(\s|$)`)

var (
	segmentLead = regexp.MustCompile(`^\s*(\w+=\S*\s+)*(sudo\s+|env\s+|time\s+|nice\s+)*`)
	tokenSplit  = regexp.MustCompile(`[\s=]+`)
	// a comma joins paths only inside a brace expansion ({a,/etc/x} is two words to the shell);
	// a colon only in a list value (PATH=/a:/b). Elsewhere both are text — `sed -n 1,/^x/p` and
	// `git show HEAD:path` name no path at the root.
	assignList = regexp.MustCompile(`^[A-Za-z_]\w*=`)
	// A quoted token is a pattern or a label, not a path — OutsideWorkspace skips it, so
	// `awk '/x|y/'` never reads the alternation as /x|y/ on disk.
	tokenTrail = regexp.MustCompile(`[)\]}>;,.]+$`)
	tildePath  = regexp.MustCompile(`^~(/|$)`)
	homeVar    = regexp.MustCompile(`^\$(HOME|\{HOME\})(/|$)`)
	absPath    = regexp.MustCompile(`^/[^/]`)
	climbPath  = regexp.MustCompile(`(^|/)\.\.(/|$)`)
	cdCmd      = regexp.MustCompile(`^cd\s`)
	netDevice  = regexp.MustCompile(`/dev/(tcp|udp)/`)
	redirectTo = regexp.MustCompile(`(?:^|[^>&0-9])[0-9]?>>?\|?\s*([^\s;&|<>]+)`)
	// the commands whose arguments are write targets: all of them, or the last
	writesAll  = regexp.MustCompile(`^(tee|touch|mkdir|chmod|chown|chgrp|truncate|rm|rmdir|shred)\s`)
	writesLast = regexp.MustCompile(`^(cp|mv|install|ln|rsync|sed\s+(-\S*i|--in-place))\s`)
)

// writeTargets lists the paths a segment writes: redirect targets, and the arguments of the
// commands that write to theirs.
func writeTargets(seg string) []string {
	var out []string
	for _, m := range redirectTo.FindAllStringSubmatch(seg, -1) {
		out = append(out, m[1])
	}
	var args []string
	for _, f := range strings.Fields(redirectTo.ReplaceAllString(seg, " "))[1:] {
		if !strings.HasPrefix(f, "-") {
			args = append(args, strings.Trim(f, `"'`))
		}
	}
	switch {
	case writesAll.MatchString(seg):
		out = append(out, args...)
	case writesLast.MatchString(seg) && len(args) > 0:
		out = append(out, args[len(args)-1])
	}
	return out
}

// Segments is a command split at its pipes and separators, each part's leading assignments and
// wrappers (sudo, env, time, nice) dropped. A | inside quotes is a pattern's alternation
// (`grep -E 'a|b' file`), not a pipe: separators inside '…' or "…" stay text.
func Segments(cmd string) []string { return segments(cmd) }

func segments(cmd string) []string {
	// `|` not followed by `|`: the regexp consumes the next char, so re-split by hand.
	var parts []string
	cur := strings.Builder{}
	rs := []rune(cmd)
	var quote rune
	for i := 0; i < len(rs); i++ {
		c := rs[i]
		switch {
		case quote != 0:
			cur.WriteRune(c)
			if c == quote {
				quote = 0
			}
		case c == '\'' || c == '"':
			quote = c
			cur.WriteRune(c)
		case c == '|' && i+1 < len(rs) && rs[i+1] == '|', c == '&' && i+1 < len(rs) && rs[i+1] == '&':
			parts = append(parts, cur.String())
			cur.Reset()
			i++
		case c == '|' && i > 0 && rs[i-1] == '>':
			cur.WriteRune(c) // `>|` is a redirection, not a pipe
		case c == '&' && !(i > 0 && (rs[i-1] == '>' || rs[i-1] == '<')) && !(i+1 < len(rs) && rs[i+1] == '>'):
			// a lone & ends a command and starts the next; `2>&1`, `>&2` and `&>` are redirections
			parts = append(parts, cur.String())
			cur.Reset()
		case c == '|' || c == ';' || c == '\n':
			parts = append(parts, cur.String())
			cur.Reset()
		default:
			cur.WriteRune(c)
		}
	}
	parts = append(parts, cur.String())
	return lead(parts)
}

func lead(parts []string) []string {
	var out []string
	for _, p := range parts {
		s := strings.TrimSpace(segmentLead.ReplaceAllString(p, ""))
		if s != "" {
			out = append(out, s)
		}
	}
	return out
}

// OutsideWorkspace returns the first token of cmd that names a path outside the workspace, or "".
func (e Env) OutsideWorkspace(cmd string) string {
	home, _ := os.UserHomeDir()
	for _, tok := range pathWords(cmd) {
		if tok == "" {
			continue
		}
		if c := tok[0]; c == '\'' || c == '"' || c == '`' {
			continue // a quoted token is a pattern or a label, not a path
		}
		tok = strings.TrimLeft(tok, "<") // `cat </etc/hosts` reads the path after the redirection
		p := ""
		switch {
		case tildePath.MatchString(tok):
			p = filepath.Join(home, tok[1:])
		case homeVar.MatchString(tok):
			p = filepath.Join(home, homeVar.ReplaceAllString(tok, "$2"))
		case absPath.MatchString(tok) || tok == "/":
			p = filepath.Clean(tok)
		case climbPath.MatchString(tok):
			p = filepath.Clean(filepath.Join(e.Dir(), tok))
		}
		if p != "" && !e.Inside(tokenTrail.ReplaceAllString(p, "")) {
			return tok
		}
	}
	return ""
}

// pathWords are the words of cmd that may name a path: split at spaces and =, a brace expansion
// expanded as the shell does, an assignment's list value split at its colons.
func pathWords(cmd string) []string {
	var out []string
	for _, f := range strings.Fields(cmd) {
		parts := tokenSplit.Split(f, -1)
		if assignList.MatchString(f) {
			parts = append(parts[:1], strings.Split(strings.Join(parts[1:], "="), ":")...)
		}
		for _, p := range parts {
			out = append(out, braces(p)...)
		}
	}
	return out
}

// braces expands a word's brace expansions as the shell does — /etc/{hosts,passwd} is
// /etc/hosts and /etc/passwd, prefix and suffix kept on every alternative, nested groups
// included. A word with no comma group is itself.
func braces(w string) []string {
	open, depth := -1, 0
	var commas []int
	for i := 0; i < len(w); i++ {
		switch w[i] {
		case '{':
			if depth == 0 {
				open, commas = i, nil
			}
			depth++
		case ',':
			if depth == 1 {
				commas = append(commas, i)
			}
		case '}':
			if depth == 0 {
				continue
			}
			if depth--; depth == 0 && len(commas) > 0 {
				pre, post := w[:open], w[i+1:]
				var out []string
				from := open + 1
				for _, c := range append(commas, i) {
					out = append(out, braces(pre+w[from:c]+post)...)
					from = c + 1
				}
				return out
			}
		}
	}
	return []string{w}
}

// ClassifyCommand reads a shell command and returns the strongest class any segment reaches,
// with the reason.
func (e Env) ClassifyCommand(cmd string) Classification {
	best := Classification{Class: Read, Why: "read-only"}
	bump := func(c Class, why string) {
		if c > best.Class {
			best = Classification{Class: c, Why: why}
		}
	}
	if esc := e.OutsideWorkspace(cmd); esc != "" {
		bump(Outward, "path outside the workspace: "+esc)
	}
	if m := netDevice.FindString(cmd); m != "" {
		bump(Outward, "network through "+m)
	}
	// command words first: a wrapper (`env -i /bin/rm`) is still there to show where the command is
	for _, seg := range segments(guard.CommandWords(cmd)) {
		seg = guard.CommandWords(seg)
		for _, t := range writeTargets(seg) {
			if strings.HasPrefix(t, "/") && writesSystem(filepath.Clean(t)) {
				bump(Outward, "writes a system path: "+t)
			}
		}
		for _, r := range destructiveRules {
			if r.re.MatchString(seg) {
				bump(Destructive, r.why)
			}
		}
		for _, r := range outwardRules {
			if r.re.MatchString(seg) {
				bump(Outward, r.why)
			}
		}
		for _, r := range vocabularyWriteRules {
			if r.re.MatchString(seg) {
				bump(Outward, r.why)
			}
		}
		for _, r := range writeRules {
			if r.re.MatchString(seg) {
				bump(Write, r.why)
			}
		}
		if !readOnly.MatchString(seg) && !cdCmd.MatchString(seg) {
			bump(Write, "unknown command: "+strings.Fields(seg)[0])
		}
		if cdCmd.MatchString(seg) && e.OutsideWorkspace(seg[3:]) != "" {
			bump(Outward, "cd outside the workspace")
		}
	}
	return best
}

var recordPath = regexp.MustCompile(`(^|/)(AGENT-ONE\.md|log\.md|design/[^/]*\.md|notes\.jsonl)$`)

// IsRecord reports whether a path is one of the workspace's records (never overwritten: Policy 4).
func IsRecord(p string) bool { return recordPath.MatchString(filepath.ToSlash(p)) }

// ClassifyPath settles the class of a file write: outside the workspace is outward, onto a record
// is destructive, else write.
func (e Env) ClassifyPath(abs string) Classification {
	if !e.Inside(abs) || writesSystem(abs) {
		return Classification{Class: Outward, Why: "path outside the workspace: " + abs, Paths: []string{abs}}
	}
	if e.isRecord(abs) {
		return Classification{Class: Destructive, Why: "overwrite of a record: " + e.Rel(abs), Paths: []string{abs}}
	}
	return Classification{Class: Write, Why: "writes " + e.Rel(abs), Paths: []string{abs}}
}

// wrappers run the command that follows them; a permission rule is matched after each.
var wrappers = regexp.MustCompile(`^(?:(?:sudo|env|nice|time|nohup|command|exec|eval|xargs|timeout)\b\s*|(?:sh|bash|zsh|dash)\s+-c\s+)`)

// gitLead is `git` with at least one global option before the verb.
var gitLead = regexp.MustCompile(`^git(?:\s+` + gitOpt + `)+\s+`)

// CommandForms lists the shapes a permission rule is also matched against, so a rule on
// `git push*` holds when the same act hides behind env, sudo, xargs, a quoted `sh -c`
// argument, a `cd … &&` prefix, a subshell, or git's global options. Arguments are not
// commands: `echo git push` yields no `git push`. A rule matched on a form only tightens
// (deny or ask) — an allow still needs the whole command (app.decideHook).
func CommandForms(cmd string) []string {
	whole := strings.TrimSpace(cmd)
	seen := map[string]bool{}
	var out []string
	var explore func(s string)
	explore = func(s string) {
		s = strings.TrimSpace(s)
		if s == "" || seen[s] {
			return
		}
		seen[s] = true
		if s != whole {
			out = append(out, s)
		}
		if m := gitLead.FindString(s); m != "" {
			explore("git " + s[len(m):])
		}
		if m := wrappers.FindString(s); m != "" {
			explore(s[len(m):])
		}
		// a wrapper's own options (`sudo -u x`, `timeout 5`): one token stripped, then two
		if f := strings.Fields(s); len(f) > 1 && (strings.HasPrefix(f[0], "-") || f[0][0] >= '0' && f[0][0] <= '9') {
			explore(strings.Join(f[1:], " "))
			if len(f) > 2 && strings.HasPrefix(f[0], "-") {
				explore(strings.Join(f[2:], " "))
			}
		}
		// a quoted or subshell command: what follows the opener, and the same closed again
		if i := strings.IndexAny(s, "'\"`("); i >= 0 {
			rest := s[i+1:]
			explore(rest)
			explore(strings.TrimRight(rest, "'\"`) "))
		}
	}
	for _, seg := range segments(cmd) {
		explore(seg)
	}
	return out
}
