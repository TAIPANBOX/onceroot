// Command onceroot asks oncerootd to run one command as root after a person
// approves it, and lists, approves or rejects waiting requests.
package main

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/TAIPANBOX/onceroot/internal/client"
	"github.com/TAIPANBOX/onceroot/internal/journal"
	"github.com/TAIPANBOX/onceroot/internal/mcp"
	"github.com/TAIPANBOX/onceroot/internal/wire"
)

const usage = `usage:
  onceroot run [--why TEXT] -- PROGRAM [ARG...]   ask to run one command as root
  onceroot list [--json]                         requests onceroot holds
  onceroot status                                onceroot's state
  onceroot approve ID                            approve a request (polkit asks you)
  onceroot reject ID                             reject a request
  onceroot journal verify                        check the signed journal
  onceroot mcp                                   MCP server with one tool, run_as_root
  onceroot bar                                   status for an Omarchy bar module (JSON)
  onceroot review                                approve or reject waiting requests one by one`

// exitRefused is the exit code of a request that did not run.
const exitRefused = 126

type invocation struct {
	cmd  string
	argv []string
	why  string
	id   string
	json bool
}

func parseArgs(args []string) (invocation, error) {
	if len(args) == 0 {
		return invocation{}, errors.New("no command")
	}
	inv := invocation{cmd: args[0]}
	rest := args[1:]
	switch inv.cmd {
	case "run":
		for len(rest) > 0 && rest[0] != "--" {
			if rest[0] != "--why" || len(rest) < 2 {
				return inv, fmt.Errorf("run: unexpected %q; the command goes after --", rest[0])
			}
			inv.why, rest = rest[1], rest[2:]
		}
		if len(rest) < 2 {
			return inv, errors.New("run: give the command after --")
		}
		inv.argv = rest[1:]
	case "list":
		if len(rest) > 1 || (len(rest) == 1 && rest[0] != "--json") {
			return inv, errors.New("list takes only --json")
		}
		inv.json = len(rest) == 1
	case "status", "mcp", "bar", "review":
		if len(rest) != 0 {
			return inv, fmt.Errorf("%s takes no arguments", inv.cmd)
		}
	case "approve", "reject":
		if len(rest) != 1 || !wire.ValidID(rest[0]) {
			return inv, fmt.Errorf("%s takes one request id, five letters", inv.cmd)
		}
		inv.id = rest[0]
	case "journal":
		if len(rest) != 1 || rest[0] != "verify" {
			return inv, errors.New("journal takes verify")
		}
		inv.cmd = "journal verify"
	default:
		return inv, fmt.Errorf("unknown command %q", inv.cmd)
	}
	return inv, nil
}

// env is everything do touches outside its arguments.
type env struct {
	sock, journal, pub string
	stdin              io.Reader
	stdout, stderr     io.Writer
}

func main() {
	inv, err := parseArgs(os.Args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "onceroot: %v\n%s\n", err, usage)
		os.Exit(2)
	}
	os.Exit(do(inv, env{
		sock:    client.DefaultSocket,
		journal: "/var/lib/onceroot/journal",
		pub:     "/var/lib/onceroot/journal.pub",
		stdin:   os.Stdin, stdout: os.Stdout, stderr: os.Stderr,
	}))
}

func do(inv invocation, e env) int {
	fail := func(err error) int {
		fmt.Fprintln(e.stderr, err)
		return 1
	}
	switch inv.cmd {
	case "run":
		code, err := client.Run(e.sock, inv.argv, inv.why, e.stdout, e.stderr)
		if err != nil {
			fmt.Fprintln(e.stderr, err)
			if errors.Is(err, client.ErrRefused) {
				return exitRefused
			}
			return 1
		}
		return code
	case "list":
		ev, err := client.Do(e.sock, wire.Request{Op: "list"})
		if err != nil {
			return fail(err)
		}
		if inv.json {
			json.NewEncoder(e.stdout).Encode(ev.Cards)
			return 0
		}
		for _, c := range ev.Cards {
			fmt.Fprintf(e.stdout, "%s %s\n%s\n\n", c.ID, c.Status, c.Card)
		}
		return 0
	case "status":
		ev, err := client.Do(e.sock, wire.Request{Op: "status"})
		if err != nil {
			return fail(err)
		}
		for _, line := range ev.Status {
			fmt.Fprintln(e.stdout, line)
		}
		return 0
	case "approve", "reject":
		if _, err := client.Do(e.sock, wire.Request{Op: inv.cmd, ID: inv.id}); err != nil {
			return fail(err)
		}
		fmt.Fprintf(e.stdout, "%s %s\n", inv.id, map[string]string{"approve": "approved", "reject": "rejected"}[inv.cmd])
		return 0
	case "bar":
		ev, err := client.Do(e.sock, wire.Request{Op: "list"})
		if err != nil {
			b, _ := json.Marshal(map[string]string{"text": barIcon + " !", "tooltip": "oncerootd is not reachable", "class": "active"})
			fmt.Fprintln(e.stdout, string(b))
			return 0
		}
		if b := barJSON(ev.Cards); b != "" {
			fmt.Fprintln(e.stdout, b)
		}
		return 0
	case "review":
		ev, err := client.Do(e.sock, wire.Request{Op: "list"})
		if err != nil {
			return fail(err)
		}
		review(ev.Cards, e.stdin, e.stdout, func(op, id string) error {
			_, err := client.Do(e.sock, wire.Request{Op: op, ID: id})
			return err
		})
		return 0
	case "journal verify":
		pub, err := os.ReadFile(e.pub)
		if err != nil || len(pub) != ed25519.PublicKeySize {
			return fail(fmt.Errorf("onceroot: cannot read the journal key %s", e.pub))
		}
		n, err := journal.Verify(e.journal, ed25519.PublicKey(pub))
		if err != nil {
			return fail(err)
		}
		fmt.Fprintf(e.stdout, "journal verified: %d entries\n", n)
		return 0
	case "mcp":
		relay := func(ctx context.Context, argv []string, why string) mcp.Result {
			var out, errb bytes.Buffer
			code, err := client.RunContext(ctx, e.sock, argv, why, &out, &errb)
			r := mcp.Result{Stdout: out.String(), Stderr: errb.String(), Code: code}
			if err != nil {
				r.Refused = err.Error()
			}
			return r
		}
		if err := mcp.Serve(e.stdin, e.stdout, relay); err != nil {
			return fail(err)
		}
		return 0
	}
	return 2
}
