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
	// a comma joins paths only inside a brace expansion ({a,/etc/x} is two words to the shell);
	// a colon only in a list value (PATH=/a:/b). Elsewhere both are text — `sed -n 1,/^x/p` and
	// `git show HEAD:path` name no path at the root.
	assignList = regexp.MustCompile(`^[A-Za-z_]\w*=`)
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
		out = append(out, strings.Trim(m[1], `"'`))
	}
	words := strings.Fields(redirectTo.ReplaceAllString(seg, " "))
	if len(words) == 0 {
		return out // a bare redirection (`>x`) names its target and runs nothing
	}
	var args []string
	for _, f := range words[1:] {
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

// Targets reads the paths a shell command may write — every segment's redirect targets and the
// arguments of the commands that write to theirs, the same read inside a wrapper or a quoted
// `sh -c` — resolved as absolute paths inside the workspace. A relative target is resolved against
// the working dir and against every dir a `cd` in the command names; a symlinked target is
// named where it points too; a glob adds what it matches. It is a reading, not a report: it
// errs toward naming too much, and it feeds a refusal of harness soil, never an ownership claim.
func (e Env) Targets(cmd string) []string {
	cmd = guard.CommandWords(cmd)
	segs := segments(cmd)
	dirs := []string{e.Dir()}
	for _, seg := range segs {
		if seg = strings.TrimLeft(seg, "({ \t"); cdCmd.MatchString(seg) {
			if f := strings.Fields(seg); len(f) > 1 {
				if d := e.expandTarget(strings.Trim(f[1], `"'()`)); d != "" {
					if !filepath.IsAbs(d) {
						d = filepath.Join(dirs[len(dirs)-1], d)
					}
					dirs = append(dirs, d)
				}
			}
		}
	}
	seen := map[string]bool{}
	var out []string
	add := func(abs string) {
		if abs = filepath.Clean(abs); !seen[abs] && e.insideLiteral(abs) {
			seen[abs] = true
			out = append(out, abs)
		}
	}
	for _, seg := range append(segs, CommandForms(cmd)...) {
		for _, t := range writeTargets(guard.CommandWords(seg)) {
			t = e.expandTarget(strings.TrimRight(t, `)"'`+"`"))
			if t == "" {
				continue
			}
			var abs []string
			if filepath.IsAbs(t) {
				abs = []string{t}
			} else {
				for _, d := range dirs {
					abs = append(abs, filepath.Join(d, t))
				}
			}
			for _, a := range abs {
				add(a)
				add(realOf(a))
				if strings.ContainsAny(a, "*?[") {
					ms, _ := filepath.Glob(a)
					for _, m := range ms {
						add(m)
						add(realOf(m))
					}
				}
			}
		}
	}
	return out
}

// expandTarget expands what a target names before its path: ~, $HOME, $AGENT_ONE_ROOT (the root a
// shell tool's command sees) and $PWD. A target any other variable or substitution builds
// cannot be read and is "".
func (e Env) expandTarget(t string) string {
	home, _ := os.UserHomeDir()
	for _, v := range []struct{ name, val string }{{"AGENT_ONE_ROOT", e.Root}, {"PWD", e.Dir()}, {"HOME", home}} {
		t = strings.ReplaceAll(t, "${"+v.name+"}", v.val)
		if strings.HasPrefix(t, "$"+v.name) {
			if rest := t[len(v.name)+1:]; rest == "" || rest[0] == '/' {
				t = v.val + rest
			}
		}
	}
	if tildePath.MatchString(t) {
		t = filepath.Join(home, t[1:])
	}
	if t == "" || strings.ContainsAny(t, "$`") || harmless(t) {
		return ""
	}
	return t
}

func harmless(p string) bool {
	for _, d := range harmlessDevices {
		if p == d || strings.HasSuffix(d, "/") && strings.HasPrefix(p, d) {
			return true
		}
	}
	return false
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

// OutsideWorkspace returns the first word of cmd that names a path outside the workspace, or "".
func (e Env) OutsideWorkspace(cmd string) string {
	home, _ := os.UserHomeDir()
	for _, tok := range pathWords(cmd) {
		tok = strings.TrimLeft(tok, "<") // `cat </etc/hosts` reads the path after the redirection
		if tok == "" {
			continue
		}
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

// pathWords are the words of cmd that may name a path: the shell's words (shellWords, quotes
// removed), each split at = with an assignment's list value split at its colons, and a brace
// expansion expanded as the shell does. A quoted word counts whole — `cat '/etc/my file'` names
// /etc/my file — and, when it holds more than one word, so do the words inside it, for a shell
// may run it (`bash -c 'cat /etc/x'`). The one exception is a pattern tool's program, quoted:
// it is read in the tool's own language (programPaths), never as shell.
func pathWords(cmd string) []string {
	var out []string
	var pt *patternTool
	tool, prog, expect, operands := "", false, 0, false
	for _, w := range shellWords(cmd, true) {
		if w.first {
			tool, pt, prog, expect, operands = "", nil, false, 0, false
		}
		switch {
		case tool == "" && !assignList.MatchString(w.text):
			tool = filepath.Base(w.text)
			if p, ok := patternTools[tool]; ok { // after assignments only: a wrapper's command is unread
				pt = &p
			}
		case pt == nil:
		case expect == 1, expect == 0 && !prog && (operands || !strings.HasPrefix(w.text, "-")):
			prog, expect = true, 0
			if w.quoted && !w.bare && !w.expands {
				out = append(out, programPaths(pt.lang, w.text)...)
				continue
			}
		case expect != 0:
			expect = 0
		case operands:
		case w.text == "--":
			operands = true
		case strings.HasPrefix(w.text, "-") && len(w.text) > 1:
			if k, next := pt.option(w.text); k != 0 {
				prog = prog || k != 3
				if next {
					expect = k
				}
			}
		}
		out = append(out, wordPaths(w)...)
	}
	return out
}

// wordPaths is one word's path candidates: the word split at = (an assignment's list value at
// its colons too), each part brace-expanded, and a quoted word's inner words besides.
func wordPaths(w shellWord) []string {
	var out []string
	if w.quoted && strings.ContainsAny(w.text, " \t\n") {
		out = append(out, pathWords(w.text)...)
	}
	parts := strings.Split(w.text, "=")
	if assignList.MatchString(w.text) {
		parts = append(parts[:1], strings.Split(strings.Join(parts[1:], "="), ":")...)
	}
	for _, p := range parts {
		out = append(out, braces(p)...)
	}
	return out
}

// A patternTool takes its first operand — or an -e value — as a program in its own language:
// a regex (grep), a sed script, an awk program, a jq filter. prog are the options whose value
// is the program, file those naming a program file (after them every operand is a file), vals
// the other options that take a value.
type patternTool struct{ lang, prog, file, vals string }

var patternTools = func() map[string]patternTool {
	grep := patternTool{"regex", "-e --regexp", "-f --file", "-A -B -C -m -d -D --after-context --before-context --context --max-count --label --binary-files --devices --directories --exclude --exclude-from --exclude-dir --include --color --colour"}
	rg := patternTool{"regex", "-e --regexp", "-f --file", "-A -B -C -m -M -g -t -T -j -r -E --glob --iglob --type --type-not --threads --replace --encoding --max-count --max-columns --after-context --before-context --context --max-depth --sort --sortr --colors --type-add --pre --pre-glob --ignore-file --path-separator"}
	sed := patternTool{"sed", "-e --expression", "-f --file", "-l --line-length"}
	awk := patternTool{"awk", "", "-f --file", "-F -v --field-separator --assign"}
	jq := patternTool{"jq", "", "-f --from-file", "--arg --argjson --slurpfile --rawfile --indent -L"}
	return map[string]patternTool{"grep": grep, "egrep": grep, "fgrep": grep, "rg": rg,
		"sed": sed, "awk": awk, "gawk": awk, "mawk": awk, "nawk": awk, "jq": jq}
}()

// option says what an option word takes — 1 the program, 2 a program file, 3 another value, 0
// nothing known — and whether that value is the next word. A short cluster is read letter by
// letter: the first letter that takes a value takes the rest of the word, or the next word
// (-rf F, -e'x', --file=F).
func (t *patternTool) option(w string) (kind int, next bool) {
	find := func(name string) int {
		for k, list := range []string{t.prog, t.file, t.vals} {
			for _, o := range strings.Fields(list) {
				if o == name {
					return k + 1
				}
			}
		}
		return 0
	}
	if strings.HasPrefix(w, "--") {
		name, _, attached := strings.Cut(w, "=")
		return find(name), !attached
	}
	for i := 1; i < len(w); i++ {
		if k := find("-" + w[i:i+1]); k != 0 {
			return k, i == len(w)-1
		}
	}
	return 0, false
}

// sedFile is a sed command that reads or writes a file named in the same word: r, R, w or W
// leading the word or after its address (1r/f, $w/f, /re/r/f). sedExec finds a command that runs
// a shell command in a program: e, or an s command's e flag (read broadly: a false find only
// reads the program as shell).
var (
	sedFile = regexp.MustCompile(`^(?:[0-9]+|\$|/[^/]*/)?[rRwW](/.*)$`)
	sedExec = regexp.MustCompile(`(^|[\s;{}0-9$/])e(\s|;|$)|[[:punct:]][gpIiMm0-9]*e[gpIiMm0-9]*\s*(;|}|$)`)
)

// programPaths are the paths a pattern tool's program names. A regex names none. In sed, the
// file of an r, R, w or W command does — the word after the command, or the rest of a word like
// 1r/etc/x — and a program that runs a shell command (sedExec) is read as shell throughout. In
// awk, only its strings do ("/etc/x" to getline, print > or system), each read as a quoted word;
// a bare /re/ is a regex. In jq, the module an import or include names.
func programPaths(lang, prog string) []string {
	var out []string
	if lang == "sed" && sedExec.MatchString(prog) {
		return wordPaths(shellWord{text: prog, quoted: true})
	}
	words := shellWords(prog, false)
	for i, w := range words {
		prev := ""
		if i > 0 {
			prev = words[i-1].text
		}
		switch {
		case lang == "sed" && sedFile.MatchString(w.text):
			out = append(out, sedFile.FindStringSubmatch(w.text)[1])
		case lang == "sed" && prev != "" && strings.ContainsAny(prev[len(prev)-1:], "rRwW"),
			lang == "awk" && w.quoted,
			lang == "jq" && (prev == "import" || prev == "include"):
			out = append(out, wordPaths(shellWord{text: w.text, quoted: true})...)
		}
	}
	return out
}

// A shellWord is one word of a command as the shell splits it — at whitespace and, with ops, at
// ; | & ( ) ` and newlines outside quotes — with its quotes and escapes removed: `'/etc/my file'`
// is the one word /etc/my file. first marks a segment's first word; quoted a word with a quoted
// part; bare one with a part outside quotes (where the shell still globs and brace-expands);
// expands one whose double quotes hold a $ or ` the shell substitutes, or a $'…' escape — its
// text is not what runs. A ~ or $ that leads a word inside quotes is not expanded, so the word
// reads as ./~… — a name inside the workspace.
type shellWord struct {
	text                         string
	first, quoted, bare, expands bool
}

func shellWords(cmd string, ops bool) []shellWord {
	var out []shellWord
	var cur strings.Builder
	w := shellWord{first: true}
	in := false
	flush := func() {
		if in {
			w.text = cur.String()
			out = append(out, w)
			w = shellWord{}
		}
		cur.Reset()
		in = false
	}
	// lead writes a quoted byte; a ~ (or a single-quoted $) that leads the word is a name
	lead := func(d byte, single bool) {
		if cur.Len() == 0 && (d == '~' || single && d == '$') {
			cur.WriteString("./")
		}
		cur.WriteByte(d)
	}
	for i := 0; i < len(cmd); i++ {
		switch c := cmd[i]; {
		case ops && (c == '\n' || strings.IndexByte(";|&()`", c) >= 0):
			flush()
			w.first = true
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			flush()
		case c == '\'' || c == '$' && i+1 < len(cmd) && cmd[i+1] == '\'':
			ansi := c == '$' // $'…': its escapes can spell anything
			if ansi {
				i++
			}
			in, w.quoted = true, true
			j := strings.IndexByte(cmd[i+1:], '\'')
			if j < 0 {
				j = len(cmd) - i - 1 // unterminated: the shell refuses it; read it to the end
			}
			lit := cmd[i+1 : i+1+j]
			w.expands = w.expands || ansi && strings.Contains(lit, "\\")
			for k := 0; k < len(lit); k++ {
				lead(lit[k], true)
			}
			i += j + 1
		case c == '"':
			in, w.quoted = true, true
			for i++; i < len(cmd) && cmd[i] != '"'; i++ {
				d := cmd[i]
				if d == '\\' && i+1 < len(cmd) && strings.IndexByte("\\\"$`\n", cmd[i+1]) >= 0 {
					i++
					d = cmd[i]
				} else if d == '$' || d == '`' {
					w.expands = true
				}
				lead(d, false)
			}
		case c == '\\' && i+1 < len(cmd):
			in, w.bare = true, true
			i++
			cur.WriteByte(cmd[i])
		default:
			in, w.bare = true, true
			cur.WriteByte(c)
		}
	}
	flush()
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
	best.Targets = e.Targets(cmd)
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
