package codex

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"brainwash/internal/ir"
)

func TestWriteSessionMetaHasRequiredFields(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd := filepath.Join(home, "proj")
	_ = os.MkdirAll(cwd, 0o755)
	sess := &ir.Session{
		ID: "src", Slot: ir.SlotPi, CWD: cwd, Title: "hello",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Events: []ir.Event{
			{Timestamp: time.Now(), Role: ir.RoleUser, Text: "hi"},
			{Timestamp: time.Now(), Role: ir.RoleAssistant, Text: "yo"},
		},
	}
	path, err := Parser{}.Write(sess, cwd, ir.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	first := strings.SplitN(string(raw), "\n", 2)[0]
	var obj map[string]any
	if err := json.Unmarshal([]byte(first), &obj); err != nil {
		t.Fatal(err)
	}
	if obj["type"] != "session_meta" {
		t.Fatalf("first type=%v line=%s", obj["type"], first)
	}
	payload, _ := obj["payload"].(map[string]any)
	for _, k := range []string{"id", "session_id", "cwd", "originator", "cli_version", "source", "timestamp"} {
		if payload[k] == nil || payload[k] == "" {
			t.Fatalf("missing %s in %s", k, first)
		}
	}
	if payload["originator"] != "Codex Desktop" {
		t.Fatalf("originator=%v", payload["originator"])
	}
}

func TestWriteEmitsDesktopChatEvents(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd := filepath.Join(home, "proj")
	_ = os.MkdirAll(cwd, 0o755)
	sess := &ir.Session{
		ID: "src", Slot: ir.SlotPi, CWD: cwd, Title: "hello",
		CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Events: []ir.Event{
			{Timestamp: time.Now(), Role: ir.RoleUser, Text: "hi from user"},
			{Timestamp: time.Now(), Role: ir.RoleAssistant, Text: "yo from asst"},
		},
	}
	path, err := Parser{}.Write(sess, cwd, ir.WriteOptions{})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	body := string(raw)
	for _, need := range []string{`"type":"event_msg"`, `"type":"user_message"`, `"type":"agent_message"`, `"type":"task_started"`, `"type":"task_complete"`, `hi from user`, `yo from asst`} {
		if !strings.Contains(body, need) {
			t.Fatalf("missing %s in\n%s", need, body[:min(len(body), 800)])
		}
	}
}

func TestRepairSessionMetaFillsRequiredFields(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "rollout-x.jsonl")
	thin := `{"payload":{"cwd":"/tmp/p","id":"7dc61c06-00f8-425b-843f-ee4dc9620857","source":"brainwash","timestamp":"2026-09-01T03:38:34.683Z"},"timestamp":"2026-09-01T03:38:34.683Z","type":"session_meta"}` + "\n"
	rest := `{"payload":{"content":[{"text":"hi","type":"input_text"}],"role":"user","type":"message"},"timestamp":"2026-09-01T03:38:34.683Z","type":"response_item"}` + "\n"
	if err := os.WriteFile(path, []byte(thin+rest), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := RepairSessionMeta(path); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(path)
	first := strings.SplitN(string(raw), "\n", 2)[0]
	var obj map[string]any
	_ = json.Unmarshal([]byte(first), &obj)
	payload, _ := obj["payload"].(map[string]any)
	if payload["originator"] != "Codex Desktop" || payload["cli_version"] == nil || payload["session_id"] == nil {
		t.Fatalf("repaired=%s", first)
	}
	if !strings.Contains(string(raw), `"type":"response_item"`) {
		t.Fatalf("lost body")
	}
}
