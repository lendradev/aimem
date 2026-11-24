package cli

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/lendradev/aimem/internal/engine"
	"github.com/lendradev/aimem/internal/memory"
)

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

type rpcResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Result  any             `json:"result,omitempty"`
	Error   *rpcError       `json:"error,omitempty"`
}

type toolDef struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	InputSchema map[string]any `json:"inputSchema"`
}

// runMCP serves the Model Context Protocol over stdio using newline delimited JSON.
func runMCP(eng *engine.Engine) int {
	tools := []toolDef{
		{
			Name:        "memory_recall",
			Description: "Search stored memory for facts, decisions, procedures, and past outcomes. Use this before asking the user to repeat context.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"query":  map[string]any{"type": "string", "description": "What to look for."},
					"limit":  map[string]any{"type": "integer", "description": "Max items, default 5."},
					"tokens": map[string]any{"type": "integer", "description": "Token budget for the response, default 600."},
					"scope":  map[string]any{"type": "string", "enum": []string{"all", "project", "global"}, "description": "Which scope to search."},
					"full":   map[string]any{"type": "boolean", "description": "Return full bodies."},
				},
				"required": []string{"query"},
			},
		},
		{
			Name:        "memory_remember",
			Description: "Store one memory item. Use for durable facts, decisions, procedures, and preferences worth recalling in later sessions. Write the title and the body in English; keep technical terms, file names, error messages, and shell commands exactly as they are. Set fundamental for knowledge that stays valid across projects and years: prune never drops it.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"title":       map[string]any{"type": "string", "description": "One sentence in English, max 120 bytes."},
					"body":        map[string]any{"type": "string", "description": "Optional detail in English, max 32 KB."},
					"kind":        map[string]any{"type": "string", "enum": []string{"fact", "decision", "procedure", "preference", "lesson", "dead_end", "correction"}, "description": "Item type, default fact."},
					"tags":        map[string]any{"type": "array", "items": map[string]any{"type": "string"}, "description": "Up to 8 lowercase tags."},
					"scope":       map[string]any{"type": "string", "enum": []string{"project", "global"}, "description": "Target scope, default project."},
					"fundamental": map[string]any{"type": "boolean", "description": "Keep permanently. Prune never drops fundamental items, while non fundamental items become stale memory after years without recall."},
				},
				"required": []string{"title"},
			},
		},
		{
			Name:        "memory_feedback",
			Description: "Report whether a remembered item turned out useful, a dead end, or was wrong. This is what makes memory improve over time.",
			InputSchema: map[string]any{
				"type": "object",
				"properties": map[string]any{
					"id":         map[string]any{"type": "string", "description": "Item id from memory_recall."},
					"outcome":    map[string]any{"type": "string", "enum": []string{"useful", "dead_end", "corrected"}},
					"correction": map[string]any{"type": "string", "description": "What the correct answer was, when outcome is corrected."},
				},
				"required": []string{"id", "outcome"},
			},
		},
	}

	scanner := bufio.NewScanner(os.Stdin)
	scanner.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	enc := json.NewEncoder(os.Stdout)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		var req rpcRequest
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			writeRPC(enc, rpcResponse{JSONRPC: "2.0", Error: &rpcError{Code: -32700, Message: "parse error: " + err.Error()}})
			continue
		}
		resp := handleRPC(eng, req, tools)
		if resp == nil {
			continue
		}
		writeRPC(enc, *resp)
	}
	return 0
}

func writeRPC(enc *json.Encoder, resp rpcResponse) {
	enc.SetEscapeHTML(false)
	if err := enc.Encode(resp); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
	}
}

func handleRPC(eng *engine.Engine, req rpcRequest, tools []toolDef) *rpcResponse {
	resp := &rpcResponse{JSONRPC: "2.0", ID: req.ID}
	switch req.Method {
	case "initialize":
		resp.Result = map[string]any{
			"protocolVersion": "2024-11-05",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]any{"name": currentAlias(), "version": versionNumber},
		}
	case "notifications/initialized", "initialized":
		return nil
	case "tools/list":
		resp.Result = map[string]any{"tools": tools}
	case "ping":
		resp.Result = map[string]any{}
	case "tools/call":
		var params struct {
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			resp.Error = &rpcError{Code: -32602, Message: "invalid params: " + err.Error()}
			return resp
		}
		result, err := callTool(eng, params.Name, params.Arguments)
		if err != nil {
			resp.Error = &rpcError{Code: -32603, Message: err.Error()}
			return resp
		}
		resp.Result = result
	default:
		resp.Error = &rpcError{Code: -32601, Message: "method not found: " + req.Method}
	}
	return resp
}

func callTool(eng *engine.Engine, name string, args json.RawMessage) (any, error) {
	switch name {
	case "memory_recall":
		var a struct {
			Query  string `json:"query"`
			Limit  int    `json:"limit"`
			Tokens int    `json:"tokens"`
			Scope  string `json:"scope"`
			Full   bool   `json:"full"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, err
		}
		scope := a.Scope
		if scope == "" {
			scope = "all"
		}
		res, err := eng.Recall(engine.RecallOptions{
			Query: a.Query, Scope: scope, Limit: a.Limit, TokenBudget: a.Tokens, Full: a.Full,
		})
		if err != nil {
			return nil, err
		}
		return toolText(formatResults(res)), nil

	case "memory_remember":
		var a struct {
			Title       string   `json:"title"`
			Body        string   `json:"body"`
			Kind        string   `json:"kind"`
			Tags        []string `json:"tags"`
			Scope       string   `json:"scope"`
			Fundamental bool     `json:"fundamental"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, err
		}
		kind := a.Kind
		if kind == "" {
			kind = string(memory.KindFact)
		}
		scope := a.Scope
		if scope == "" {
			scope = memory.ScopeProject
		}
		res, err := eng.Remember(engine.RememberOptions{
			Scope: scope, Kind: kind, Title: a.Title, Body: a.Body, Tags: a.Tags,
			Source: "mcp", Fundamental: a.Fundamental,
		})
		if err != nil {
			return nil, err
		}
		if res.DuplicateOf != nil {
			return toolText(fmt.Sprintf("not stored, similar item exists: %s (id %s, similarity %.2f)",
				res.DuplicateOf.Title, res.DuplicateOf.ID, res.DuplicateScore)), nil
		}
		it := res.Item
		return toolText(fmt.Sprintf("saved %s (%s, scope %s)", it.ID, it.Kind, it.Scope)), nil

	case "memory_feedback":
		var a struct {
			ID         string `json:"id"`
			Outcome    string `json:"outcome"`
			Correction string `json:"correction"`
		}
		if err := json.Unmarshal(args, &a); err != nil {
			return nil, err
		}
		it, err := eng.Feedback(a.ID, memory.Outcome(a.Outcome), a.Correction, "mcp")
		if err != nil {
			return nil, err
		}
		return toolText(fmt.Sprintf("%s -> status %s, score %.4f", it.ID, it.Status, it.Score)), nil
	}
	return nil, fmt.Errorf("unknown tool %q", name)
}

func toolText(text string) map[string]any {
	return map[string]any{
		"content": []map[string]any{{"type": "text", "text": text}},
	}
}

func formatResults(res []engine.RecallResult) string {
	if len(res) == 0 {
		return "no match"
	}
	var b strings.Builder
	for _, r := range res {
		fmt.Fprintf(&b, "- [%s] %s (id %s, score %.2f, scope %s)\n", r.Kind, r.Title, r.ID, r.Score, r.Scope)
		if r.Body != "" {
			for _, line := range strings.Split(r.Body, "\n") {
				b.WriteString("  " + line + "\n")
			}
		}
		if r.Truncated {
			fmt.Fprintf(&b, "  (truncated, show %s for the rest)\n", r.ID)
		}
	}
	return strings.TrimRight(b.String(), "\n")
}
