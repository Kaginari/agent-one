# zone-shell

- **Rank:** Zone worker
- **Territory:** `shell/`, `sandbox/`
- **Reports to:** domain-safety
- **Skills:** zone-go-package, zone-act-classes
- **Purpose:** the ground truth of containment: one persistent bash per agent, run under bubblewrap with a scrubbed environment.

## Traits
- One bash per agent (`--noprofile --norc`), started under the sandbox. Every command is framed by a heredoc and a per-call nonce sentinel (`__ISK_HD_<id>` / `__ISK_END_<id>`) that carries the exit code and the resulting cwd; a command containing the delimiter is refused. An EXIT trap prints the sentinel when the shell itself exits, and the session restarts.
- A timeout kills the whole process tree the command started (descendants read from /proc), sparing the shell's own group; `Restart` throws the session away. Background jobs are named (`^[A-Za-z0-9][A-Za-z0-9._-]{0,63}$`), logged to files, listed, killed on Close. Output is capped in bytes and the clip is reported.
- Network: the persistent shell runs with `--unshare-net`; a command the gate approved as outward runs in a one-shot net-enabled shell seeded with the session's exports and cwd.
- bwrap arguments: `--ro-bind / /`, `--tmpfs /tmp`, `--dev /dev`, `--proc /proc`, `--die-with-parent`, `--new-session`, `--bind <root> <root>`, each RWPath bound only if it exists, `--chdir`. Where bwrap is missing or cannot run (user namespaces refused) Mode is None and Why is kept for `status`; macOS has no bwrap.
- `ScrubEnv` drops `(^|_)(API_KEY|TOKEN|SECRET|PASSWORD|PASSWD)$` case-insensitively plus EnvDrop globs; EnvAllow wins over every drop; EnvSet (the package registries) wins over the process env.
- Tests exercise Bwrap mode only where bwrap runs (this machine has /usr/bin/bwrap); CI installs bubblewrap and relaxes the apparmor userns sysctl first.

## Verify
- `go test ./shell/ ./sandbox/`

## Working notes
