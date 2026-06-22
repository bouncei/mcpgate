package jsonrpc

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
)

// JSON-RPC 2.0 error codes (negative reserved range) plus one server-defined code.
const (
	CodeParseError       = -32700
	CodeInvalidRequest   = -32600
	CodeMethodNotFound   = -32601
	CodeInvalidParams    = -32602
	CodeToolNotPermitted = -32001 // server-defined: blocked by allowlist
)

// Message is a single JSON-RPC request or notification.
type Message struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// Parse decodes a single JSON-RPC message. Batched (array) bodies are rejected
// because MCP 2025-06-18 removed batching support.
func Parse(body []byte) (*Message, error) {
	trimmed := bytes.TrimSpace(body)
	if len(trimmed) == 0 {
		return nil, errors.New("empty request body")
	}
	if trimmed[0] == '[' {
		return nil, errors.New("JSON-RPC batching is not supported")
	}
	var m Message
	if err := json.Unmarshal(trimmed, &m); err != nil {
		return nil, fmt.Errorf("invalid JSON-RPC message: %w", err)
	}
	return &m, nil
}

// ToolName extracts params.name for a tools/call message.
func (m *Message) ToolName() (string, bool) {
	if len(m.Params) == 0 {
		return "", false
	}
	var p struct {
		Name string `json:"name"`
	}
	if err := json.Unmarshal(m.Params, &p); err != nil || p.Name == "" {
		return "", false
	}
	return p.Name, true
}

type errorObject struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type errorEnvelope struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Error   errorObject     `json:"error"`
}

// ErrorResponse builds a JSON-RPC error response body.
func ErrorResponse(id json.RawMessage, code int, msg string) []byte {
	if len(id) == 0 {
		id = json.RawMessage("null")
	}
	b, _ := json.Marshal(errorEnvelope{
		JSONRPC: "2.0",
		ID:      id,
		Error:   errorObject{Code: code, Message: msg},
	})
	return b
}

// FilterToolsList removes tools the caller may not use from a tools/list
// response body, preserving all other fields. Responses without result.tools
// (e.g. error responses) pass through unchanged.
func FilterToolsList(body []byte, allowed func(name string) bool) ([]byte, error) {
	var root map[string]json.RawMessage
	if err := json.Unmarshal(body, &root); err != nil {
		return nil, err
	}
	rawResult, ok := root["result"]
	if !ok {
		return body, nil
	}
	var result map[string]json.RawMessage
	if err := json.Unmarshal(rawResult, &result); err != nil {
		return nil, err
	}
	rawTools, ok := result["tools"]
	if !ok {
		return body, nil
	}
	var tools []map[string]json.RawMessage
	if err := json.Unmarshal(rawTools, &tools); err != nil {
		return nil, err
	}
	kept := make([]map[string]json.RawMessage, 0, len(tools))
	for _, tool := range tools {
		var name string
		if raw, ok := tool["name"]; ok {
			_ = json.Unmarshal(raw, &name)
		}
		if allowed(name) {
			kept = append(kept, tool)
		}
	}
	keptRaw, err := json.Marshal(kept)
	if err != nil {
		return nil, err
	}
	result["tools"] = keptRaw
	resultRaw, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	root["result"] = resultRaw
	return json.Marshal(root)
}

// FilterToolsListSSE applies FilterToolsList to every `data:` payload in a
// Server-Sent Events stream (the form an MCP server may return tools/list in).
// Non-JSON data lines and all other SSE fields (event:, id:, comments, blank
// lines) are preserved verbatim, so event framing and resumability survive.
func FilterToolsListSSE(body []byte, allowed func(name string) bool) ([]byte, error) {
	var out bytes.Buffer
	lines := bytes.Split(body, []byte("\n"))
	for i, raw := range lines {
		line := raw
		hasCR := len(line) > 0 && line[len(line)-1] == '\r'
		if hasCR {
			line = line[:len(line)-1]
		}
		if bytes.HasPrefix(line, []byte("data:")) {
			payload := bytes.TrimSpace(line[len("data:"):])
			if len(payload) > 0 {
				// FilterToolsList returns the payload unchanged when it has no
				// result.tools, and errors only on malformed JSON — in which
				// case this isn't a tool list, so keep the line verbatim.
				if filtered, err := FilterToolsList(payload, allowed); err == nil {
					out.WriteString("data: ")
					out.Write(filtered)
					if hasCR {
						out.WriteByte('\r')
					}
					if i < len(lines)-1 {
						out.WriteByte('\n')
					}
					continue
				}
			}
		}
		out.Write(raw)
		if i < len(lines)-1 {
			out.WriteByte('\n')
		}
	}
	return out.Bytes(), nil
}
