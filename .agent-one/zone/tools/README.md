# zone-tools

- **Rank:** Zone worker
- **Territory:** `tool/`
- **Reports to:** domain-safety
- **Skills:** zone-go-package, zone-act-classes
- **Purpose:** the ground truth of the tools an agent may run and the class each call reaches.

## Traits
- `Class` is an ordered int: Read < Write < Outward < Destructive. `Tool.Class` is the floor; `Classify` may raise it; the caller's `class` input may raise it; a lower declaration is kept as a `@?` hole ("a declaration only tightens").
- `ClassifyCommand` splits a command into segments and takes the strongest: destructiveRules (rm, shred, mv, dd/mkfs, git reset/clean/rebase/gc/prune, `push --force`/`+ref`, branch -d, stash drop, find -delete, a record overwritten by `>`, `sed -i`, `tee`, `cp`), outwardRules (git remotes, curl/ssh/rsync/nc…, npm/pip/go get, docker, cloud CLIs, `https?://`, a secret store at segment start, brew), writeRules (redirects, tee/cp/mkdir/touch/chmod…, sed -i, git local writes). An unknown first word is a Write; a path outside the workspace (absolute, `~`, `$HOME`, a `../` climb) is Outward.
- `Env.Confine` walks the longest existing prefix through symlinks; a path that escapes the root through a symlink is refused. Files are written atomically (temp file + rename, mode preserved).
- Builtins: read, write, edit, multiedit, patch (unified diff), bash (the persistent shell; named background jobs), ls, glob, grep, git (argv, never a shell; class per subcommand), webfetch (http/https only, Outward, HTML reduced to text), websearch (needs a `Searcher` backend), ask, dispatch (`Commission`; `RoleOf`: findings → analyst, verdict → judge, draft → drafter), custom (argv with `{{param}}` each filling one element, or a bash template with `$P_<name>`; name `[a-z][a-z0-9_-]*`; floor Write).
- A missing tool is answered with aliases and edit-distance suggestions; `DoomLoop` stops a repeated call to the same missing name.
- Vendor declarations exist for anthropic only (bash_20250124, text_editor_20250728 as `str_replace_based_edit_tool`); every other provider sees the JSON schema.

## Verify
- `go test ./tool/`

## Working notes
