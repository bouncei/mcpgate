package audit

import (
	"bytes"
	"encoding/json"
	"testing"
	"time"
)

func TestDecisionEmitsJSON(t *testing.T) {
	var buf bytes.Buffer
	l := NewWithWriter(&buf)
	l.Decision(Event{
		Label:    "alice",
		Method:   "tools/call",
		Tool:     "read_file",
		Decision: "allow",
		Status:   200,
		Latency:  5 * time.Millisecond,
	})
	var got map[string]any
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, buf.String())
	}
	if got["label"] != "alice" || got["tool"] != "read_file" || got["decision"] != "allow" {
		t.Errorf("unexpected fields: %v", got)
	}
	if int(got["status"].(float64)) != 200 {
		t.Errorf("status = %v", got["status"])
	}
}
