package jsonrpc

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseRequest(t *testing.T) {
	m, err := Parse([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/call","params":{"name":"read_file"}}`))
	if err != nil {
		t.Fatal(err)
	}
	if m.Method != "tools/call" {
		t.Errorf("method = %q", m.Method)
	}
	name, ok := m.ToolName()
	if !ok || name != "read_file" {
		t.Errorf("tool name = %q ok=%v", name, ok)
	}
}

func TestParseRejectsBatch(t *testing.T) {
	_, err := Parse([]byte(`[{"jsonrpc":"2.0","id":1,"method":"x"}]`))
	if err == nil {
		t.Fatal("expected batch to be rejected")
	}
}

func TestParseRejectsGarbage(t *testing.T) {
	if _, err := Parse([]byte(`not json`)); err == nil {
		t.Fatal("expected parse error")
	}
	if _, err := Parse([]byte(``)); err == nil {
		t.Fatal("expected empty error")
	}
}

func TestToolNameMissing(t *testing.T) {
	m, _ := Parse([]byte(`{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}`))
	if _, ok := m.ToolName(); ok {
		t.Fatal("expected no tool name for tools/list")
	}
}

func TestErrorResponse(t *testing.T) {
	b := ErrorResponse(json.RawMessage(`7`), CodeToolNotPermitted, "nope")
	var env map[string]any
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatal(err)
	}
	if env["jsonrpc"] != "2.0" {
		t.Errorf("jsonrpc = %v", env["jsonrpc"])
	}
	errObj := env["error"].(map[string]any)
	if int(errObj["code"].(float64)) != CodeToolNotPermitted {
		t.Errorf("code = %v", errObj["code"])
	}
}

func TestErrorResponseNilID(t *testing.T) {
	b := ErrorResponse(nil, CodeInvalidRequest, "bad")
	if !strings.Contains(string(b), `"id":null`) {
		t.Errorf("expected null id, got %s", b)
	}
}

func TestFilterToolsList(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":1,"result":{"tools":[
		{"name":"read_file","description":"r"},
		{"name":"run_command","description":"x"},
		{"name":"list_dir","description":"l"}
	],"nextCursor":"abc"}}`)
	out, err := FilterToolsList(body, func(name string) bool {
		return name == "read_file" || name == "list_dir"
	})
	if err != nil {
		t.Fatal(err)
	}
	var parsed struct {
		Result struct {
			Tools      []map[string]any `json:"tools"`
			NextCursor string           `json:"nextCursor"`
		} `json:"result"`
	}
	if err := json.Unmarshal(out, &parsed); err != nil {
		t.Fatal(err)
	}
	if len(parsed.Result.Tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(parsed.Result.Tools))
	}
	if parsed.Result.NextCursor != "abc" {
		t.Errorf("nextCursor not preserved: %q", parsed.Result.NextCursor)
	}
	for _, tool := range parsed.Result.Tools {
		if tool["name"] == "run_command" {
			t.Error("run_command should have been filtered out")
		}
	}
}

func TestFilterToolsListSSE(t *testing.T) {
	body := []byte("event: message\nid: abc\ndata: {\"jsonrpc\":\"2.0\",\"id\":1,\"result\":{\"tools\":[{\"name\":\"read_file\"},{\"name\":\"run_command\"}]}}\n\n")
	out, err := FilterToolsListSSE(body, func(name string) bool { return name == "read_file" })
	if err != nil {
		t.Fatal(err)
	}
	s := string(out)
	if !strings.Contains(s, "read_file") {
		t.Error("read_file should remain")
	}
	if strings.Contains(s, "run_command") {
		t.Errorf("run_command should be filtered from SSE: %s", s)
	}
	if !strings.Contains(s, "event: message") || !strings.Contains(s, "id: abc") {
		t.Errorf("SSE framing not preserved: %s", s)
	}
	for _, line := range strings.Split(s, "\n") {
		if strings.HasPrefix(line, "data: ") {
			var m map[string]any
			if err := json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &m); err != nil {
				t.Errorf("data payload not valid JSON: %v", err)
			}
		}
	}
}

func TestFilterToolsListSSENonJSONPreserved(t *testing.T) {
	body := []byte("event: ping\ndata: not-json\n\n")
	out, err := FilterToolsListSSE(body, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(body) {
		t.Errorf("non-JSON SSE data should be preserved verbatim:\ngot:  %q\nwant: %q", out, body)
	}
}

func TestFilterToolsListErrorPassthrough(t *testing.T) {
	body := []byte(`{"jsonrpc":"2.0","id":1,"error":{"code":-32601,"message":"x"}}`)
	out, err := FilterToolsList(body, func(string) bool { return true })
	if err != nil {
		t.Fatal(err)
	}
	if string(out) != string(body) {
		t.Errorf("error response should pass through unchanged")
	}
}
