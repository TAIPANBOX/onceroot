package mcp

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func serve(t *testing.T, relay Relay, lines ...string) []map[string]any {
	t.Helper()
	var out bytes.Buffer
	if err := Serve(strings.NewReader(strings.Join(lines, "\n")+"\n"), &out, relay); err != nil {
		t.Fatal(err)
	}
	var answers []map[string]any
	for _, l := range strings.Split(strings.TrimSpace(out.String()), "\n") {
		if l == "" {
			continue
		}
		var m map[string]any
		if err := json.Unmarshal([]byte(l), &m); err != nil {
			t.Fatalf("not JSON: %q", l)
		}
		answers = append(answers, m)
	}
	return answers
}

func noRelay(ctx context.Context, argv []string, why string) Result { return Result{} }

func TestInitializeEchoesProtocol(t *testing.T) {
	a := serve(t, noRelay, `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"c","version":"1"}}}`)
	res := a[0]["result"].(map[string]any)
	if res["protocolVersion"] != "2025-06-18" || res["serverInfo"].(map[string]any)["name"] != "onceroot" {
		t.Fatalf("got %v", a[0])
	}
}

func TestToolsListHasOneTool(t *testing.T) {
	a := serve(t, noRelay, `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`)
	tools := a[0]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != 1 || tools[0].(map[string]any)["name"] != "run_as_root" {
		t.Fatalf("tools %v", tools)
	}
	schema := tools[0].(map[string]any)["inputSchema"].(map[string]any)
	if req := schema["required"].([]any); len(req) != 2 {
		t.Fatalf("schema %v", schema)
	}
}

func TestCallRelaysExitCode(t *testing.T) {
	var gotArgv []string
	var gotWhy string
	relay := func(ctx context.Context, argv []string, why string) Result {
		gotArgv, gotWhy = argv, why
		return Result{Stdout: "installed\n", Stderr: "onceroot: approve (approved by polkit)\n", Code: 3}
	}
	a := serve(t, relay, `{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"run_as_root","arguments":{"argv":["pacman","-S","x"],"why":"install x"}}}`)
	res := a[0]["result"].(map[string]any)
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	if strings.Join(gotArgv, " ") != "pacman -S x" || gotWhy != "install x" {
		t.Fatalf("relay got %v %q", gotArgv, gotWhy)
	}
	if !strings.Contains(text, "exit code 3") || !strings.Contains(text, "installed") || res["isError"] != true {
		t.Fatalf("result %v", res)
	}
}

func TestCallReportsARefusal(t *testing.T) {
	relay := func(context.Context, []string, string) Result {
		return Result{Refused: "deny: built-in: bash is a shell"}
	}
	a := serve(t, relay, `{"jsonrpc":"2.0","id":4,"method":"tools/call","params":{"name":"run_as_root","arguments":{"argv":["bash"],"why":"x"}}}`)
	res := a[0]["result"].(map[string]any)
	text := res["content"].([]any)[0].(map[string]any)["text"].(string)
	if res["isError"] != true || !strings.Contains(text, "NOT RUN") || !strings.Contains(text, "built-in") {
		t.Fatalf("result %v", res)
	}
}

func TestCallRefusesBadArguments(t *testing.T) {
	called := false
	relay := func(context.Context, []string, string) Result { called = true; return Result{} }
	a := serve(t, relay,
		`{"jsonrpc":"2.0","id":5,"method":"tools/call","params":{"name":"run_as_root","arguments":{"argv":[],"why":"x"}}}`,
		`{"jsonrpc":"2.0","id":6,"method":"tools/call","params":{"name":"other","arguments":{}}}`,
		`{"jsonrpc":"2.0","id":7,"method":"tools/call","params":{"name":"run_as_root","arguments":{"argv":"pacman -S x","why":"x"}}}`,
	)
	if called {
		t.Fatal("the relay ran for malformed arguments")
	}
	for _, m := range a {
		if m["error"] == nil {
			t.Fatalf("no error for %v", m)
		}
	}
}

func TestUnknownMethodIsJSONRPCError(t *testing.T) {
	a := serve(t, noRelay, `{"jsonrpc":"2.0","id":8,"method":"resources/list"}`, `not json`)
	if a[0]["error"].(map[string]any)["code"] != float64(-32601) {
		t.Fatalf("got %v", a[0])
	}
	if a[1]["error"].(map[string]any)["code"] != float64(-32700) {
		t.Fatalf("got %v", a[1])
	}
}

func TestNotificationGetsNoAnswer(t *testing.T) {
	a := serve(t, noRelay, `{"jsonrpc":"2.0","method":"notifications/initialized"}`, `{"jsonrpc":"2.0","id":9,"method":"ping"}`)
	if len(a) != 1 || a[0]["id"] != float64(9) {
		t.Fatalf("answers %v", a)
	}
}

func TestCancelledCallStopsTheRelay(t *testing.T) {
	stopped := make(chan struct{})
	relay := func(ctx context.Context, argv []string, why string) Result {
		select {
		case <-ctx.Done():
			close(stopped)
		case <-time.After(5 * time.Second):
		}
		return Result{Stdout: "late"}
	}
	a := serve(t, relay,
		`{"jsonrpc":"2.0","id":11,"method":"tools/call","params":{"name":"run_as_root","arguments":{"argv":["pacman","-S","x"],"why":"x"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/cancelled","params":{"requestId":11,"reason":"user stopped"}}`,
		`{"jsonrpc":"2.0","id":12,"method":"ping"}`,
	)
	select {
	case <-stopped:
	case <-time.After(2 * time.Second):
		t.Fatal("the relay was not told the call was cancelled")
	}
	for _, m := range a {
		if m["id"] == float64(11) {
			t.Fatalf("a cancelled call was answered: %v", m)
		}
	}
	if len(a) != 1 || a[0]["id"] != float64(12) {
		t.Fatalf("answers %v", a)
	}
}
