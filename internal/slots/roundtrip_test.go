package slots_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"brainwash/internal/ir"
	"brainwash/internal/slot"
	_ "brainwash/internal/slots/claude"
	_ "brainwash/internal/slots/codex"
	_ "brainwash/internal/slots/dsh"
	_ "brainwash/internal/slots/pi"
)

func TestSlotRegistry(t *testing.T) {
	names := slot.Names()
	if len(names) != 4 {
		t.Fatalf("slots=%v", names)
	}
}

func TestFourWayConvertNoLiveTools(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	src := sample()
	cwd := filepath.Join(home, "proj")
	_ = os.MkdirAll(cwd, 0o755)

	for _, from := range slot.Names() {
		p := slot.Must(from)
		path, err := p.Write(src, cwd, ir.WriteOptions{IncludeTools: true, MaxToolChars: 200})
		if err != nil {
			t.Fatalf("write %s: %v", from, err)
		}
		refs, err := p.List(cwd)
		if err != nil || len(refs) == 0 {
			t.Fatalf("list %s path=%s refs=%v err=%v", from, path, refs, err)
		}
		loaded, err := p.Load(refs[0])
		if err != nil {
			t.Fatalf("load %s: %v", from, err)
		}
		if len(loaded.Events) < 2 {
			t.Fatalf("%s events=%d", from, len(loaded.Events))
		}
		hasUser, hasAsst := false, false
		for _, ev := range loaded.Events {
			if ev.Role == ir.RoleUser && strings.Contains(ev.Text, "fix the login") {
				hasUser = true
			}
			if ev.Role == ir.RoleAssistant && (strings.Contains(ev.Text, "patched") || strings.Contains(ev.Text, "Action")) {
				hasAsst = true
			}
		}
		if !hasUser || !hasAsst {
			t.Fatalf("%s missing dialogue user=%v asst=%v events=%+v", from, hasUser, hasAsst, titles(loaded))
		}
		for _, to := range slot.Names() {
			if to == from {
				continue
			}
			out := filepath.Join(home, "out-"+string(from)+"-"+string(to))
			_ = os.MkdirAll(out, 0o755)
			dst := slot.Must(to)
			wpath, err := dst.Write(loaded, out, ir.WriteOptions{IncludeTools: true})
			if err != nil {
				t.Fatalf("%s->%s write: %v", from, to, err)
			}
			raw, err := os.ReadFile(wpath)
			if err != nil {
				// zstd destination still exists
				if _, statErr := os.Stat(wpath); statErr != nil {
					t.Fatalf("%s->%s missing %s: %v", from, to, wpath, err)
				}
			}
			body := string(raw)
			if strings.Contains(body, `"type":"toolCall"`) || strings.Contains(body, `"type":"tool_use"`) || strings.Contains(body, `"type":"function_call"`) {
				t.Fatalf("%s->%s leaked live tool protocol in %s", from, to, wpath)
			}
			drefs, err := dst.List(out)
			if err != nil || len(drefs) == 0 {
				t.Fatalf("%s->%s list empty", from, to)
			}
			again, err := dst.Load(drefs[0])
			if err != nil {
				t.Fatalf("%s->%s load: %v", from, to, err)
			}
			if len(again.Events) == 0 {
				t.Fatalf("%s->%s empty after load", from, to)
			}
		}
	}
}

func sample() *ir.Session {
	now := time.Date(2026, 8, 31, 10, 0, 0, 0, time.UTC)
	return &ir.Session{
		ID: "src-1", Slot: ir.SlotPi, CWD: "/tmp/proj", Title: "login bug",
		SourcePath: "/tmp/src.jsonl", CreatedAt: now, UpdatedAt: now,
		Events: []ir.Event{
			{Timestamp: now, Role: ir.RoleUser, Kind: ir.KindInput, Text: "please fix the login timeout", SourceKind: "test.user"},
			{Timestamp: now.Add(time.Minute), Role: ir.RoleAssistant, Text: "patched AuthService retry", Thinking: "need to inspect timeout", Tools: []ir.ToolTrace{
				{CallID: "c1", Name: "read", Arguments: `{"path":"AuthService.swift"}`, Result: "class AuthService"},
			}, SourceKind: "test.assistant"},
		},
	}
}

func titles(s *ir.Session) []string {
	var out []string
	for _, ev := range s.Events {
		out = append(out, ev.Role+":"+ev.Text[:min(40, len(ev.Text))])
	}
	return out
}
