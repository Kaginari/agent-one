# zone-yaml

- **Rank:** Zone worker
- **Territory:** `config/yaml/`
- **Reports to:** domain-config
- **Skills:** zone-go-package
- **Purpose:** the ground truth of the configuration syntax: the strict YAML subset, its JSON twin, and the in-place editor `config patch` uses.

## Traits
- The subset: block maps, block lists, one-line flow `{}`/`[]`, plain / 'single' / "double" scalars, bools, ints, floats, null, `#` comments, `|` and `>` block scalars. Anchors, aliases, tags, directives, multiple documents, complex keys, merge keys and tab indentation are errors carrying file:line — by design, not by omission.
- Every `Node` carries File and Line (`Where()`); `ParseJSON` produces the same Node kinds so a JSON config gets the same origins in `config explain`.
- Plain scalars are typed only as `true`/`false` (three casings), `null`/`~`/empty, ints and floats; every other bare word is a string.
- `Emit`, `Edit` and `Diff` rewrite a file in place for `config patch`, preserving the key column and comments outside the edited entry; `Equal` compares trees structurally.

## Verify
- `go test ./config/yaml/`

## Working notes
