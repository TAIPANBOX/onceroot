// Package rules decides what happens to a request before anyone is asked:
// a built-in deny list no rule can override, then the owner's rules file,
// first match wins, and otherwise a person is asked.
package rules

import (
	"bufio"
	"fmt"
	"io"
	"strings"
)

// Verdict is what happens to a request.
type Verdict int

const (
	Ask Verdict = iota
	Allow
	Deny
)

func (v Verdict) String() string {
	switch v {
	case Allow:
		return "allow"
	case Deny:
		return "deny"
	}
	return "ask"
}

// Rule is one line of the rules file:
//
//	allow|deny <absolute path> [<arg pattern>...] [...]
//
// A pattern is a literal or ends in "*" (prefix match). A trailing "..."
// lets further arguments through; without it the argument count must match.
type Rule struct {
	Line    int
	Verdict Verdict
	Path    string
	Args    []string
	Rest    bool
}

// Parse reads a rules file. Every error names its line.
func Parse(r io.Reader) ([]Rule, error) {
	var out []Rule
	sc := bufio.NewScanner(r)
	for n := 1; sc.Scan(); n++ {
		rule, ok, err := parseLine(sc.Text(), n)
		if err != nil {
			return nil, err
		}
		if ok {
			out = append(out, rule)
		}
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("rules: %w", err)
	}
	return out, nil
}

func parseLine(line string, n int) (Rule, bool, error) {
	bad := func(format string, a ...any) (Rule, bool, error) {
		return Rule{}, false, fmt.Errorf("rules: line %d: %s", n, fmt.Sprintf(format, a...))
	}
	for _, r := range line {
		if (r < 0x20 && r != '\t') || r == 0x7f {
			return bad("control character %U", r)
		}
	}
	if i := commentStart(line); i >= 0 {
		line = line[:i]
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return Rule{}, false, nil
	}
	rule := Rule{Line: n}
	switch fields[0] {
	case "allow":
		rule.Verdict = Allow
	case "deny":
		rule.Verdict = Deny
	default:
		return bad("unknown verb %q (want allow or deny)", fields[0])
	}
	if len(fields) < 2 {
		return bad("no program path")
	}
	rule.Path = fields[1]
	if !strings.HasPrefix(rule.Path, "/") {
		return bad("program path %q is not absolute", rule.Path)
	}
	if strings.Contains(rule.Path, "*") {
		return bad("program path %q holds a *", rule.Path)
	}
	args := fields[2:]
	for i, p := range args {
		if p == "..." {
			if i != len(args)-1 {
				return bad("... must be the last word")
			}
			rule.Rest = true
			continue
		}
		if j := strings.Index(p, "*"); j >= 0 && j != len(p)-1 {
			return bad("pattern %q: only a trailing * is supported", p)
		}
		rule.Args = append(rule.Args, p)
	}
	return rule, true, nil
}

// commentStart finds a # that begins the line or follows whitespace.
func commentStart(line string) int {
	for i, r := range line {
		if r == '#' && (i == 0 || line[i-1] == ' ' || line[i-1] == '\t') {
			return i
		}
	}
	return -1
}

func (r Rule) matches(path string, argv []string) bool {
	if r.Path != path {
		return false
	}
	var args []string
	if len(argv) > 0 {
		args = argv[1:]
	}
	if len(args) < len(r.Args) || (!r.Rest && len(args) != len(r.Args)) {
		return false
	}
	for i, p := range r.Args {
		if prefix, ok := strings.CutSuffix(p, "*"); ok {
			if !strings.HasPrefix(args[i], prefix) {
				return false
			}
		} else if args[i] != p {
			return false
		}
	}
	return true
}

// Decide applies the built-in deny list, then the rules in order. path is
// the program as root resolved it; argv[0] is the name the agent used.
func Decide(rules []Rule, path string, argv []string) (Verdict, string) {
	if deny, why := Builtin(path, argv); deny {
		return Deny, "built-in: " + why
	}
	for _, r := range rules {
		if r.matches(path, argv) {
			return r.Verdict, fmt.Sprintf("rules line %d", r.Line)
		}
	}
	return Ask, "no rule matches"
}
