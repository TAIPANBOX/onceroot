package main

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/TAIPANBOX/onceroot/internal/wire"
)

// Omarchy's bar shows the raw output when the JSON text is empty, so an idle
// module prints nothing at all (VM 2026-10-27).
func TestBarSilentWhenNothingWaits(t *testing.T) {
	if out := barJSON([]wire.Listed{{ID: "KQWPT", Status: "ran"}}); out != "" {
		t.Fatalf("got %q, want nothing", out)
	}
}

// Omarchy highlights a command module only for the class "active".
func TestBarShowsCountAndActive(t *testing.T) {
	cards := []wire.Listed{{ID: "KQWPT", Status: "held", Card: "ROOT REQUEST KQWPT"}, {ID: "ABCDE", Status: "held", Card: "ROOT REQUEST ABCDE"}, {ID: "ZZZZZ", Status: "rejected"}}
	var m map[string]string
	if err := json.Unmarshal([]byte(barJSON(cards)), &m); err != nil {
		t.Fatal(err)
	}
	if m["text"] != "root? 2" || m["class"] != "active" || !strings.Contains(m["tooltip"], "KQWPT") || strings.Contains(m["tooltip"], "ZZZZZ") {
		t.Fatalf("got %v", m)
	}
}

func TestReviewApprovesAndRejects(t *testing.T) {
	cards := []wire.Listed{{ID: "KQWPT", Status: "held", Card: "A"}, {ID: "ABCDE", Status: "held", Card: "B"}, {ID: "FGHJK", Status: "held", Card: "C"}, {ID: "LMNPQ", Status: "ran", Card: "D"}}
	var acts []string
	var out strings.Builder
	review(cards, strings.NewReader("a\nr\ns\n"), &out, func(op, id string) error { acts = append(acts, op+" "+id); return nil })
	if strings.Join(acts, ",") != "approve KQWPT,reject ABCDE" {
		t.Fatalf("actions %v", acts)
	}
	for _, want := range []string{"KQWPT: approved", "ABCDE: rejected", "FGHJK: skipped"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("review does not say %q:\n%s", want, out.String())
		}
	}
	if strings.Contains(out.String(), "D") && strings.Contains(out.String(), "LMNPQ") {
		t.Fatal("a finished card was offered")
	}
}

func TestReviewNeedsAnExplicitAnswer(t *testing.T) {
	cards := []wire.Listed{{ID: "KQWPT", Status: "held", Card: "A"}}
	var acts []string
	var out strings.Builder
	review(cards, strings.NewReader("yes please\n\n"), &out, func(op, id string) error { acts = append(acts, op); return nil })
	if len(acts) != 0 {
		t.Fatalf("an unclear answer acted: %v", acts)
	}
}

// A dismissed polkit dialog fails the approval; the person must see that.
func TestReviewSaysWhenAnActionFails(t *testing.T) {
	cards := []wire.Listed{{ID: "KQWPT", Status: "held", Card: "A"}}
	var out strings.Builder
	review(cards, strings.NewReader("a\n"), &out, func(op, id string) error { return errors.New("not approved") })
	if !strings.Contains(out.String(), "approve KQWPT failed: not approved") || strings.Contains(out.String(), "KQWPT: approved") {
		t.Fatalf("review hid a failed approval:\n%s", out.String())
	}
}
