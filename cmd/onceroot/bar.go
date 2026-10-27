package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"github.com/TAIPANBOX/onceroot/internal/wire"
)

// barJSON is the Waybar-style status an Omarchy custom bar module reads: ""
// when no request waits, since Omarchy shows the raw output when the JSON text
// is empty; otherwise the count and the cards, with the class "active", the
// one Omarchy highlights.
func barJSON(cards []wire.Listed) string {
	var held []string
	for _, c := range cards {
		if c.Status == "held" {
			held = append(held, c.Card)
		}
	}
	if len(held) == 0 {
		return ""
	}
	b, _ := json.Marshal(map[string]string{
		"text":    fmt.Sprintf("root? %d", len(held)),
		"tooltip": strings.Join(held, "\n\n") + "\n\nClick to review.",
		"class":   "active",
	})
	return string(b)
}

// review walks the held cards and acts on an explicit a or r; anything else
// skips. Approval itself still goes through polkit in oncerootd.
func review(cards []wire.Listed, in io.Reader, out io.Writer, act func(op, id string) error) {
	sc := bufio.NewScanner(in)
	n := 0
	for _, c := range cards {
		if c.Status != "held" {
			continue
		}
		n++
		fmt.Fprintf(out, "%s\n\n[a]pprove (polkit will ask you), [r]eject, anything else skips: ", c.Card)
		if !sc.Scan() {
			return
		}
		var op, done string
		switch strings.TrimSpace(sc.Text()) {
		case "a":
			op, done = "approve", "approved"
		case "r":
			op, done = "reject", "rejected"
		default:
			fmt.Fprintf(out, "%s: skipped\n\n", c.ID)
			continue
		}
		if err := act(op, c.ID); err != nil {
			fmt.Fprintf(out, "%s %s failed: %v\n\n", op, c.ID, err)
			continue
		}
		fmt.Fprintf(out, "%s: %s\n\n", c.ID, done)
	}
	if n == 0 {
		fmt.Fprintln(out, "No request waits.")
	}
}
