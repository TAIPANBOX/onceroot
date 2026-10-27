package main

import (
	"strings"
	"testing"
)

func TestParseRun(t *testing.T) {
	inv, err := parseArgs([]string{"run", "--why", "install rg", "--", "pacman", "-S", "--noconfirm", "ripgrep"})
	if err != nil || inv.cmd != "run" || inv.why != "install rg" || strings.Join(inv.argv, " ") != "pacman -S --noconfirm ripgrep" {
		t.Fatalf("got %+v, %v", inv, err)
	}
}

func TestParseRunNeedsDoubleDash(t *testing.T) {
	for _, args := range [][]string{{"run", "pacman", "-S", "x"}, {"run", "--"}, {"run", "--why"}, {"run", "--colour", "x", "--", "true"}} {
		if _, err := parseArgs(args); err == nil {
			t.Errorf("%q accepted", args)
		}
	}
}

func TestParseOtherCommands(t *testing.T) {
	ok := map[string][]string{
		"list":   {"list", "--json"},
		"status": {"status"},
		"ap":     {"approve", "KQWPT"},
		"rj":     {"reject", "KQWPT"},
		"jv":     {"journal", "verify"},
		"mcp":    {"mcp"},
	}
	for name, args := range ok {
		if _, err := parseArgs(args); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
	bad := [][]string{{}, {"exec", "x"}, {"approve"}, {"approve", "KQWPI"}, {"journal"}, {"status", "x"}}
	for _, args := range bad {
		if _, err := parseArgs(args); err == nil {
			t.Errorf("%q accepted", args)
		}
	}
}
