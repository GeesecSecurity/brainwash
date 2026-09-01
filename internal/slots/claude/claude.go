package claude

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"

	"brainwash/internal/injects"
	"brainwash/internal/ir"
	"brainwash/internal/jsonl"
	"brainwash/internal/paths"
	"brainwash/internal/slot"
	"brainwash/internal/textutil"
)

const ClaudeCodeVersion = "2.1.251"

func init() { slot.Register(Parser{}) }

type Parser struct{}

func (Parser) Name() ir.Slot { return ir.SlotClaude }
func (Parser) Label() string { return "Claude Code" }

func (Parser) List(cwd string) ([]ir.SessionRef, error) {
	if strings.TrimSpace(cwd) == "" {
		return listAllClaude()
	}
	return listClaudeDir(paths.ClaudeDir(cwd), cwd)
}

func listAllClaude() ([]ir.SessionRef, error) {
	root := paths.ClaudeProjectsRoot()
	ents, err := os.ReadDir(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []ir.SessionRef
	for _, e := range ents {
		if !e.IsDir() {
			continue
		}
		cwd := paths.DecodeProjectDir(e.Name())
		items, err := listClaudeDir(filepath.Join(root, e.Name()), cwd)
		if err != nil {
			continue
		}
		out = append(out, items...)
	}
	return out, nil
}

func listClaudeDir(dir, cwd string) ([]ir.SessionRef, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []ir.SessionRef
	for _, e := range ents {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		p := filepath.Join(dir, e.Name())
		id := strings.TrimSuffix(e.Name(), ".jsonl")
		title := ""
		created := jsonl.FileMod(p)
		sessCWD := cwd
		firstUser := ""
		for _, obj := range jsonl.PeekObjects(p, 60, 256*1024) {
			if t := jsonl.TimeOf(obj["timestamp"]); created.Equal(jsonl.FileMod(p)) && !t.IsZero() {
				created = t
			}
			if c := jsonl.String(obj["cwd"]); c != "" {
				sessCWD = c
			}
			switch jsonl.String(obj["type"]) {
			case "summary":
				if s := jsonl.String(obj["summary"]); s != "" {
					title = s
				}
			case "user":
				if firstUser == "" {
					content := jsonl.Get(obj, "message", "content")
					if arr, ok := content.([]any); ok && hasToolResult(arr) {
						break
					}
					firstUser = injects.CleanTitle(jsonl.TextContent(content))
				}
			}
		}
		if title == "" {
			title = jsonl.TitleFromUserText(firstUser)
		}
		if title == "" {
			title = id
		}
		out = append(out, ir.SessionRef{
			Slot: ir.SlotClaude, ID: id, CWD: sessCWD, Title: title, Path: p,
			CreatedAt: created, UpdatedAt: jsonl.FileMod(p), Bytes: jsonl.FileSize(p),
		})
	}
	return out, nil
}

func (Parser) Load(ref ir.SessionRef) (*ir.Session, error) {
	sess := &ir.Session{
		ID: ref.ID, Slot: ir.SlotClaude, CWD: ref.CWD, Title: ref.Title,
		SourcePath: ref.Path, CreatedAt: ref.CreatedAt, UpdatedAt: ref.UpdatedAt,
		Notes: []string{"Imported from Claude Code session " + ref.ID},
	}
	lastAsst := -1
	err := jsonl.ReadLines(ref.Path, func(b []byte) error {
		var m map[string]any
		if json.Unmarshal(b, &m) != nil {
			return nil
		}
		if c := jsonl.String(m["cwd"]); c != "" {
			sess.CWD = c
		}
		ts := jsonl.TimeOf(m["timestamp"])
		if ts.IsZero() {
			ts = time.Now()
		}
		if sess.CreatedAt.IsZero() {
			sess.CreatedAt = ts
		}
		switch jsonl.String(m["type"]) {
		case "user":
			content := jsonl.Get(m, "message", "content")
			if arr, ok := content.([]any); ok && hasToolResult(arr) {
				for _, item := range arr {
					bm := jsonl.AsMap(item)
					if bm == nil || jsonl.String(bm["type"]) != "tool_result" {
						continue
					}
					callID := jsonl.String(bm["tool_use_id"])
					result := jsonl.TextContent(bm["content"])
					if result == "" {
						result = jsonl.CompactJSON(bm["content"], 4000)
					}
					isErr := jsonl.GetBool(bm, "is_error")
					if lastAsst >= 0 {
						attach(&sess.Events[lastAsst], callID, "", result, isErr)
					}
				}
				return nil
			}
			ev := injects.ClassifyUser(jsonl.TextContent(content), jsonl.ImageURLs(content))
			ev.Timestamp = ts
			ev.SourceKind = "claude.user"
			if textutil.NonEmpty(ev.Text) || len(ev.Images) > 0 || len(ev.Injects) > 0 {
				sess.Events = append(sess.Events, ev)
			}
		case "assistant":
			content := jsonl.Get(m, "message", "content")
			text, thinking, tools := parseAssistant(content)
			sess.Events = append(sess.Events, ir.Event{
				Timestamp: ts, Role: ir.RoleAssistant, Text: text,
				Thinking: thinking, Tools: tools, SourceKind: "claude.assistant",
				Interrupted: jsonl.GetBool(m, "isInterrupted") || jsonl.GetBool(m, "interrupted"),
			})
			lastAsst = len(sess.Events) - 1
		case "summary":
			if s := jsonl.String(m["summary"]); s != "" {
				sess.Title = s
			}
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if sess.Title == "" || sess.Title == sess.ID {
		sess.Title = ir.FirstUserTitle(sess.Events, sess.ID)
	}
	return sess, nil
}

func (Parser) Write(sess *ir.Session, outCWD string, opt ir.WriteOptions) (string, error) {
	id := uuid.New().String()
	path := filepath.Join(paths.ClaudeDir(outCWD), id+".jsonl")
	if opt.DryRun {
		return path, nil
	}
	now := time.Now().UTC()
	parent := ""
	add := func(obj map[string]any) {
		uid := uuid.New().String()
		obj["uuid"] = uid
		if parent != "" {
			obj["parentUuid"] = parent
		} else {
			obj["parentUuid"] = nil
		}
		obj["cwd"] = outCWD
		obj["sessionId"] = id
		obj["version"] = ClaudeCodeVersion
		if _, ok := obj["timestamp"]; !ok {
			obj["timestamp"] = now.Format(time.RFC3339Nano)
		}
		parent = uid
	}
	objs := []any{}
	push := func(obj map[string]any) {
		add(obj)
		objs = append(objs, obj)
	}
	push(map[string]any{
		"type": "user",
		"message": map[string]any{
			"role": "user",
			"content": ir.ImportBanner(sess),
		},
		"userType": "external",
	})
	push(map[string]any{
		"type": "assistant",
		"message": map[string]any{
			"role": "assistant",
			"content": []any{map[string]any{"type": "text", "text": "Imported. Ready to continue from the transferred memory."}},
		},
	})
	for _, ev := range sess.Events {
		text := ir.PrefixRole(ev, ir.Narrate(ev, opt))
		ts := ev.Timestamp.UTC().Format(time.RFC3339Nano)
		if ev.Role == ir.RoleAssistant {
			content := []any{}
			if textutil.NonEmpty(ev.Thinking) {
				content = append(content, map[string]any{"type": "thinking", "thinking": ev.Thinking})
			}
			if textutil.NonEmpty(text) {
				content = append(content, map[string]any{"type": "text", "text": text})
			}
			if len(content) == 0 {
				continue
			}
			push(map[string]any{
				"type": "assistant", "timestamp": ts,
				"message": map[string]any{"role": "assistant", "content": content},
			})
			continue
		}
		if !textutil.NonEmpty(text) {
			continue
		}
		push(map[string]any{
			"type": "user", "timestamp": ts, "userType": "external",
			"message": map[string]any{"role": "user", "content": text},
		})
	}
	if err := jsonl.WriteLines(path, objs); err != nil {
		return "", err
	}
	return path, nil
}

func hasToolResult(arr []any) bool {
	for _, item := range arr {
		m := jsonl.AsMap(item)
		if m != nil && jsonl.String(m["type"]) == "tool_result" {
			return true
		}
	}
	return false
}

func parseAssistant(v any) (text, thinking string, tools []ir.ToolTrace) {
	arr, ok := v.([]any)
	if !ok {
		text = jsonl.TextContent(v)
		return
	}
	var texts []string
	for _, item := range arr {
		m := jsonl.AsMap(item)
		if m == nil {
			continue
		}
		switch jsonl.String(m["type"]) {
		case "text":
			if s := jsonl.String(m["text"]); s != "" {
				texts = append(texts, s)
			}
		case "thinking":
			if s := jsonl.String(m["thinking"]); s != "" {
				thinking = s
			} else if s := jsonl.String(m["text"]); s != "" {
				thinking = s
			}
		case "tool_use":
			tools = append(tools, ir.ToolTrace{
				CallID: jsonl.String(m["id"]), Name: jsonl.String(m["name"]),
				Arguments: jsonl.CompactJSON(m["input"], 8000),
			})
		}
	}
	text = strings.Join(texts, "\n")
	return
}

func attach(ev *ir.Event, callID, name, result string, isErr bool) {
	for i := range ev.Tools {
		if ev.Tools[i].CallID == callID {
			ev.Tools[i].Result = result
			ev.Tools[i].IsError = isErr
			if name != "" {
				ev.Tools[i].Name = name
			}
			return
		}
	}
	if name == "" {
		name = "tool"
	}
	ev.Tools = append(ev.Tools, ir.ToolTrace{CallID: callID, Name: name, Result: result, IsError: isErr})
}
