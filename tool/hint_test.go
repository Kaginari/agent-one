package tool

import "testing"

// The shell reads the file tools serve better get their read path; anything more than a plain
// print of one file gets nothing.
func TestFileReadHint(t *testing.T) {
	for cmd, want := range map[string]string{
		"cat go.mod":                     "go.mod",
		"head -n 40 tool/builtins.go":    "tool/builtins.go:1:40",
		"head -5 a.txt":                  "a.txt:1:5",
		"head a.txt":                     "a.txt:1:10",
		"tail -n 20 log.md":              "log.md:-20",
		"tail -3 'my file'":              "",
		`sed -n '120,180p' loop/loop.go`: "loop/loop.go:120:61",
		"sed -n 7p x.go":                 "x.go:7:1",
		"cat a b":                        "",
		"cat a | grep x":                 "",
		"cat *.go":                       "",
		"cat $HOME/x":                    "",
		"head -n 5 a > b":                "",
		"tail -f log":                    "",
		"tail -n +5 x":                   "",
		"sed -i 's/a/b/' x":              "",
		"sed -n '/^func/p' x.go":         "",
		"go test ./...":                  "",
	} {
		if got := FileReadHint(cmd); got != want {
			t.Errorf("%q: %q, want %q", cmd, got, want)
		}
	}
}
