package tool

import (
	"reflect"
	"testing"
)

// SplitArgv quotes as a shell would and expands nothing; a shell operator outside quotes is a
// template, not an argv, and is refused.
func TestSplitArgvShellQuoting(t *testing.T) {
	for in, want := range map[string][]string{
		`./wrangle {{path}}`:               {"./wrangle", "{{path}}"},
		`grep -rn 'a b' "c d" e\ f`:        {"grep", "-rn", "a b", "c d", "e f"},
		`echo "say \"hi\" \$HOME" 'it''s'`: {"echo", `say "hi" $HOME`, "its"},
		`printf '%s|%s' x`:                 {"printf", "%s|%s", "x"},
		`  go   test  ./...  `:             {"go", "test", "./..."},
		`jq ".items[] | .name" {{file}}`:   {"jq", ".items[] | .name", "{{file}}"},
		`sed -e 's/;/,/g' ''`:              {"sed", "-e", "s/;/,/g", ""},
	} {
		got, err := SplitArgv(in)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Errorf("SplitArgv(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, bad := range []string{`grep x | wc -l`, `a && b`, `a; b`, `cat > out`, "echo `id`", `echo $(id)`, `echo "$(id)"`, `'open`, `"open`, `trail\`} {
		if got, err := SplitArgv(bad); err == nil {
			t.Errorf("SplitArgv(%q) = %q; want a refusal", bad, got)
		}
	}
	// the round trip: QuoteArgv joins so SplitArgv splits back the same
	for _, argv := range [][]string{{"a b", "c'd", "{{x}}", "", "plain"}, {"x|y", "$HOME"}} {
		back, err := SplitArgv(QuoteArgv(argv))
		if err != nil || !reflect.DeepEqual(back, argv) {
			t.Errorf("round trip %q → %q → %q (%v)", argv, QuoteArgv(argv), back, err)
		}
	}
}

func TestInterpreterEvalShapes(t *testing.T) {
	for argv, want := range map[string]string{
		"bash -c {{cmd}}":               "bash -c",
		"/bin/sh -c x":                  "sh -c",
		"bash -lc x":                    "bash -c",
		"env FOO=1 python3 -c print(1)": "python3 -c",
		"node -e x":                     "node -e",
		"perl -ne x":                    "perl -e",
		"ruby -e x":                     "ruby -e",
		"eval {{x}}":                    "eval",
		"python3 -m pytest":             "",
		"bash ./script.sh":              "",
		"go test ./...":                 "",
	} {
		a, _ := SplitArgv(argv)
		if got := InterpreterEval(a); got != want {
			t.Errorf("InterpreterEval(%q) = %q; want %q", argv, got, want)
		}
	}
	if !ValidCustomName("ui-wrangler") || ValidCustomName("../evil") || ValidCustomName("Bad") || ValidCustomName("") {
		t.Error("ValidCustomName must be tools.custom's own rule")
	}
}
