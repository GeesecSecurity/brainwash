package dsh

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

func init() { slot.Register(Parser{}) }

type Parser struct{}

func (Parser) Name() ir.Slot { return ir.SlotDSH }
func (Parser) Label() string { return "DeepSeek Harness" }

func (Parser) List(cwd string) ([]ir.SessionRef, error) {
	if strings.TrimSpace(cwd) == "" {
		return listAllDSH()
	}
	return listDSHDir(paths.DSHDir(cwd), cwd)
}

func listAllDSH() ([]ir.SessionRef, error) {
	root := paths.DSHSessionsRoot()
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
		items, err := listDSHDir(filepath.Join(root, e.Name()), cwd)
		if err != nil {
			continue
		}
		out = append(out, items...)
	}
	return out, nil
}

func listDSHDir(dir, cwd string) ([]ir.SessionRef, error) {
	ents, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []ir.SessionRef
	for _, e := range ents {
		if !e.IsDir() || !strings.HasPrefix(e.Name(), "session-") {
			continue
		}
		p := filepath.Join(dir, e.Name(), "session.jsonl.zstd")
		if _, err := os.Stat(p); err != nil {
			continue
		}
		id := strings.TrimPrefix(e.Name(), "session-")
		title := ""
		created := jsonl.FileMod(p)
		sessCWD := cwd
		firstUser := ""
		for _, m := range jsonl.PeekZstdObjects(p, 40, 128*1024) {
			switch jsonl.String(m["type"]) {
			case "session":
				if t := jsonl.TimeOf(m["createdAt"]); !t.IsZero() {
					created = t
				}
				if c := jsonl.String(m["cwd"]); c != "" {
					sessCWD = c
				}
			case "session/title":
				if t := jsonl.GetString(m, "data", "title"); t != "" {
					title = t
				}
			case "user/message":
				if firstUser == "" {
					firstUser = injects.CleanTitle(jsonl.TextContent(jsonl.Get(m, "data", "content")))
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
			Slot: ir.SlotDSH, ID: id, CWD: sessCWD, Title: title, Path: p,
			CreatedAt: created, UpdatedAt: jsonl.FileMod(p), Bytes: jsonl.FileSize(p),
		})
	}
	return out, nil
}

func (Parser) Load(ref ir.SessionRef) (*ir.Session, error) {
	sess := &ir.Session{
		ID: ref.ID, Slot: ir.SlotDSH, CWD: ref.CWD, Title: ref.Title,
		SourcePath: ref.Path, CreatedAt: ref.CreatedAt, UpdatedAt: ref.UpdatedAt,
		Notes: []string{"Imported from DeepSeek Harness session " + ref.ID},
	}
	lastAsst := -1
	err := jsonl.ReadZstdLines(ref.Path, func(b []byte) error {
		var m map[string]any
		if json.Unmarshal(b, &m) != nil {
			return nil
		}
		typ := jsonl.String(m["type"])
		data := jsonl.GetMap(m, "data")
		if data == nil {
			data = map[string]any{}
		}
		ts := jsonl.TimeOf(m["time"])
		if ts.IsZero() {
			ts = time.Now()
		}
		switch typ {
		case "session":
			if c := jsonl.String(m["cwd"]); c != "" {
				sess.CWD = c
			}
			if t := jsonl.TimeOf(m["createdAt"]); !t.IsZero() {
				sess.CreatedAt = t
			}
		case "session/title":
			if t := jsonl.String(data["title"]); t != "" {
				sess.Title = t
			}
		case "user/message":
			ev := injects.ClassifyUser(jsonl.TextContent(data["content"]), jsonl.ImageURLs(data["content"]))
			ev.Timestamp = ts
			ev.SourceKind = "dsh.user"
			if textutil.NonEmpty(ev.Text) || len(ev.Images) > 0 || len(ev.Injects) > 0 {
				sess.Events = append(sess.Events, ev)
			}
		case "assistant/message":
			content := jsonl.Get(data, "message", "content")
			text, thinking, tools := parseAssistant(content)
			sess.Events = append(sess.Events, ir.Event{
				Timestamp: ts, Role: ir.RoleAssistant, Text: text,
				Thinking: thinking, Tools: tools, SourceKind: "dsh.assistant",
			})
			lastAsst = len(sess.Events) - 1
		case "tool/result":
			msg := jsonl.GetMap(data, "message")
			callID := jsonl.GetString(msg, "source", "callId")
			result := ""
			isErr := false
			if arr := jsonl.GetSlice(msg, "content"); len(arr) > 0 {
				first := jsonl.AsMap(arr[0])
				if first != nil {
					if callID == "" {
						callID = jsonl.String(first["toolCallId"])
					}
					result = jsonl.TextContent(first["content"])
					isErr = jsonl.GetBool(first, "isError")
				}
			}
			if result == "" {
				result = jsonl.TextContent(jsonl.Get(msg, "content"))
			}
			if lastAsst >= 0 {
				attach(&sess.Events[lastAsst], callID, "", result, isErr)
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
	dir := filepath.Join(paths.DSHDir(outCWD), "session-"+id)
	path := filepath.Join(dir, "session.jsonl.zstd")
	if opt.DryRun {
		return path, nil
	}
	now := time.Now().UnixMilli()
	objs := []any{
		map[string]any{"type": "session", "cwd": outCWD, "createdAt": now, "id": id, "time": now},
		map[string]any{"type": "session/title", "data": map[string]any{"title": ir.DisplayTitle(sess, opt)}, "time": now},
		map[string]any{"type": "user/message", "time": now, "data": map[string]any{"content": ir.ImportBanner(sess)}},
		map[string]any{"type": "assistant/message", "time": now, "data": map[string]any{
			"message": map[string]any{"role": "assistant", "content": []any{
				map[string]any{"type": "text", "text": "Imported. Ready to continue from the transferred memory."},
			}},
		}},
	}
	for _, ev := range sess.Events {
		text := ir.PrefixRole(ev, ir.Narrate(ev, opt))
		ts := ev.Timestamp.UnixMilli()
		if ts == 0 {
			ts = now
		}
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
			objs = append(objs, map[string]any{
				"type": "assistant/message", "time": ts,
				"data": map[string]any{"message": map[string]any{"role": "assistant", "content": content}},
			})
			continue
		}
		if !textutil.NonEmpty(text) {
			continue
		}
		objs = append(objs, map[string]any{
			"type": "user/message", "time": ts, "data": map[string]any{"content": text},
		})
	}
	if err := jsonl.WriteZstdLines(path, objs); err != nil {
		return "", err
	}
	updateIndexes(id, outCWD, ir.DisplayTitle(sess, opt))
	return path, nil
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
			}
		case "tool_use", "toolCall", "tool_call":
			name := jsonl.String(m["name"])
			if name == "" {
				name = jsonl.String(m["toolName"])
			}
			id := jsonl.String(m["id"])
			if id == "" {
				id = jsonl.String(m["callId"])
			}
			tools = append(tools, ir.ToolTrace{
				CallID: id, Name: name, Arguments: jsonl.CompactJSON(m["input"], 8000),
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
			return
		}
	}
	if name == "" {
		name = "tool"
	}
	ev.Tools = append(ev.Tools, ir.ToolTrace{CallID: callID, Name: name, Result: result, IsError: isErr})
}

func updateIndexes(id, cwd, title string) {
	ws := paths.DSHWorkspace()
	raw, _ := os.ReadFile(ws)
	var obj map[string]any
	if json.Unmarshal(raw, &obj) != nil || obj == nil {
		obj = map[string]any{}
	}
	list, _ := obj["workspaces"].([]any)
	found := false
	for _, w := range list {
		m := jsonl.AsMap(w)
		if m != nil && jsonl.String(m["cwd"]) == cwd {
			found = true
			break
		}
	}
	if !found {
		list = append(list, map[string]any{"cwd": cwd, "updatedAt": time.Now().UnixMilli()})
		obj["workspaces"] = list
		b, _ := json.MarshalIndent(obj, "", "  ")
		_ = os.MkdirAll(filepath.Dir(ws), 0o755)
		_ = os.WriteFile(ws, b, 0o644)
	}
	cachePath := paths.DSHProjCache()
	raw, _ = os.ReadFile(cachePath)
	var cache map[string]any
	if json.Unmarshal(raw, &cache) != nil || cache == nil {
		cache = map[string]any{}
	}
	key := paths.EncodedProjectDir(cwd)
	entry := jsonl.AsMap(cache[key])
	if entry == nil {
		entry = map[string]any{}
	}
	sessions, _ := entry["sessions"].([]any)
	sessions = append(sessions, map[string]any{"id": id, "title": title, "cwd": cwd})
	entry["sessions"] = sessions
	cache[key] = entry
	b, _ := json.MarshalIndent(cache, "", "  ")
	_ = os.MkdirAll(filepath.Dir(cachePath), 0o755)
	_ = os.WriteFile(cachePath, b, 0o644)
}
