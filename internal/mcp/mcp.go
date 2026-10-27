// Package mcp is a stdio MCP server with one tool, run_as_root, for agents
// that prefer tools to shell commands. It speaks newline-delimited JSON-RPC
// 2.0 with the standard library only and relays each call to oncerootd.
package mcp

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
)

// Result is what a relayed request produced.
type Result struct {
	Stdout, Stderr string
	Code           int
	Refused        string // non-empty when the request did not run
}

// Relay runs one request through oncerootd until it ends or ctx is cancelled.
type Relay func(ctx context.Context, argv []string, why string) Result

const toolDescription = "Run ONE command as root after a person approves it. " +
	"Give the program and its arguments as a list (no shell strings, no sudo). " +
	"A person sees the exact command and approves or rejects it; this call waits for that. " +
	"Shells, interpreters and command wrappers are always refused."

type rpcRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type rpcError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

// Serve answers requests from in until it ends. Tool calls run alongside the
// read loop, so a notifications/cancelled for one of them is read while it
// waits for a person; the cancelled call gets no answer, as MCP asks, and
// its relay is cancelled so oncerootd withdraws the card. When in ends, every
// call still waiting is cancelled.
func Serve(in io.Reader, out io.Writer, relay Relay) error {
	sc := bufio.NewScanner(in)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	var mu sync.Mutex
	enc := json.NewEncoder(out)
	write := func(v any) error {
		mu.Lock()
		defer mu.Unlock()
		return enc.Encode(v)
	}
	var wg sync.WaitGroup
	calls := map[string]context.CancelFunc{}
	cancelled := map[string]bool{}
	var callsMu sync.Mutex
	defer func() {
		callsMu.Lock()
		for _, cancel := range calls {
			cancel()
		}
		callsMu.Unlock()
		wg.Wait()
	}()
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			write(map[string]any{"jsonrpc": "2.0", "id": nil, "error": rpcError{-32700, "parse error"}})
			continue
		}
		if len(req.ID) == 0 {
			if req.Method == "notifications/cancelled" {
				var p struct {
					RequestID json.RawMessage `json:"requestId"`
				}
				json.Unmarshal(req.Params, &p)
				callsMu.Lock()
				if cancel, ok := calls[string(p.RequestID)]; ok {
					cancelled[string(p.RequestID)] = true
					cancel()
				}
				callsMu.Unlock()
			}
			continue // notifications get no answer
		}
		answer := func(result any, rerr *rpcError) {
			a := map[string]any{"jsonrpc": "2.0", "id": req.ID}
			if rerr != nil {
				a["error"] = rerr
			} else {
				a["result"] = result
			}
			write(a)
		}
		if req.Method != "tools/call" {
			answer(dispatch(context.Background(), req, relay))
			continue
		}
		ctx, cancel := context.WithCancel(context.Background())
		key := string(req.ID)
		callsMu.Lock()
		calls[key] = cancel
		callsMu.Unlock()
		wg.Add(1)
		go func(req rpcRequest) {
			defer wg.Done()
			result, rerr := dispatch(ctx, req, relay)
			callsMu.Lock()
			delete(calls, key)
			explicit := cancelled[key]
			callsMu.Unlock()
			cancel()
			if explicit {
				return // the client cancelled it: MCP sends no answer
			}
			answer(result, rerr)
		}(req)
	}
	return sc.Err()
}

func dispatch(ctx context.Context, req rpcRequest, relay Relay) (any, *rpcError) {
	switch req.Method {
	case "initialize":
		var p struct {
			ProtocolVersion string `json:"protocolVersion"`
		}
		json.Unmarshal(req.Params, &p)
		if p.ProtocolVersion == "" {
			p.ProtocolVersion = "2025-06-18"
		}
		return map[string]any{
			"protocolVersion": p.ProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": "onceroot", "version": "0.1.0"},
		}, nil
	case "ping":
		return map[string]any{}, nil
	case "tools/list":
		return map[string]any{"tools": []any{map[string]any{
			"name":        "run_as_root",
			"description": toolDescription,
			"inputSchema": map[string]any{
				"type": "object",
				"properties": map[string]any{
					"argv": map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "minItems": 1,
						"description": "program and arguments, e.g. [\"pacman\", \"-S\", \"--noconfirm\", \"ripgrep\"]"},
					"why": map[string]any{"type": "string", "description": "one line the person will read"},
				},
				"required": []string{"argv", "why"},
			},
		}}}, nil
	case "tools/call":
		var p struct {
			Name      string `json:"name"`
			Arguments struct {
				Argv []string `json:"argv"`
				Why  string   `json:"why"`
			} `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &p); err != nil {
			return nil, &rpcError{-32602, "invalid params: " + err.Error()}
		}
		if p.Name != "run_as_root" {
			return nil, &rpcError{-32602, "unknown tool " + p.Name}
		}
		if len(p.Arguments.Argv) == 0 || p.Arguments.Argv[0] == "" {
			return nil, &rpcError{-32602, "argv must name a program"}
		}
		r := relay(ctx, p.Arguments.Argv, p.Arguments.Why)
		return toolResult(r), nil
	}
	return nil, &rpcError{-32601, "method not found: " + req.Method}
}

func toolResult(r Result) map[string]any {
	var b strings.Builder
	isError := r.Refused != "" || r.Code != 0
	if r.Refused != "" {
		fmt.Fprintf(&b, "NOT RUN: %s\n", r.Refused)
	} else {
		fmt.Fprintf(&b, "exit code %d\n", r.Code)
	}
	if r.Stdout != "" {
		b.WriteString("--- stdout\n" + r.Stdout)
	}
	if r.Stderr != "" {
		b.WriteString("\n--- stderr\n" + r.Stderr)
	}
	return map[string]any{"content": []any{map[string]any{"type": "text", "text": b.String()}}, "isError": isError}
}
