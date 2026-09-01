package pi

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

func (Parser) Name() ir.Slot  { return ir.SlotPi }
func (Parser) Label() string  { return "pi" }

func (Parser) List(cwd string) ([]ir.SessionRef, error) {
	if strings.TrimSpace(cwd) == "" {
		return listAllPi()
	}
	return listPiDir(paths.PiDir(cwd), cwd)
}

func listAllPi() ([]ir.SessionRef, error) {
	root := paths.PiSessionsRoot()
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
		items, err := listPiDir(filepath.Join(root, e.Name()), cwd)
		if err != nil {
			continue
		}
		out = append(out, items...)
	}
	return out, nil
}

func listPiDir(dir, cwd string) ([]ir.SessionRef, error) {
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
		id := parsePiID(e.Name())
		title := ""
		created := jsonl.FileMod(p)
		sessCWD := cwd
		firstUser := ""
		for _, m := range jsonl.PeekObjects(p, 40, 128*1024) {
			switch jsonl.String(m["type"]) {
			case "session":
				if t := jsonl.TimeOf(m["timestamp"]); !t.IsZero() {
					created = t
				}
				if c := jsonl.String(m["cwd"]); c != "" {
					sessCWD = c
				}
			case "session_info":
				if n := jsonl.String(m["name"]); n != "" {
					title = n
				}
			case "message":
				msg := jsonl.GetMap(m, "message")
				if jsonl.String(msg["role"]) == "user" && firstUser == "" {
					firstUser = injects.CleanTitle(jsonl.TextContent(msg["content"]))
				}
			}
		}
		if title == "" {
			if hit := jsonl.ScanFor(p, 400, 512*1024, func(m map[string]any) bool {
				return jsonl.String(m["type"]) == "session_info" && jsonl.String(m["name"]) != ""
			}); hit != nil {
				title = jsonl.String(hit["name"])
			}
		}
		if title == "" {
			title = jsonl.TitleFromUserText(firstUser)
		}
		if title == "" {
			title = id
		}
		out = append(out, ir.SessionRef{
			Slot: ir.SlotPi, ID: id, CWD: sessCWD, Title: title, Path: p,
			CreatedAt: created, UpdatedAt: jsonl.FileMod(p), Bytes: jsonl.FileSize(p),
		})
	}
	return out, nil
}

func (Parser) Load(ref ir.SessionRef) (*ir.Session, error) {
	sess := &ir.Session{
		ID: ref.ID, Slot: ir.SlotPi, CWD: ref.CWD, Title: ref.Title,
		SourcePath: ref.Path, CreatedAt: ref.CreatedAt, UpdatedAt: ref.UpdatedAt,
		Notes: []string{"Imported from pi session " + ref.ID},
	}
	lastAsst := -1
	err := jsonl.ReadLines(ref.Path, func(b []byte) error {
		var m map[string]any
		if json.Unmarshal(b, &m) != nil {
			return nil
		}
		typ := jsonl.String(m["type"])
		switch typ {
		case "session":
			if c := jsonl.String(m["cwd"]); c != "" {
				sess.CWD = c
			}
			if t := jsonl.TimeOf(m["timestamp"]); !t.IsZero() {
				sess.CreatedAt = t
			}
		case "session_info":
			if n := jsonl.String(m["name"]); n != "" {
				sess.Title = n
			}
		case "compaction":
			if s := jsonl.String(m["summary"]); s != "" {
				sess.Events = append(sess.Events, ir.Event{
					Timestamp: jsonl.TimeOf(m["timestamp"]), Role: ir.RoleSummary,
					Kind: ir.KindCompaction, Text: s, SourceKind: "pi.compaction",
				})
			}
		case "message":
			msg := jsonl.GetMap(m, "message")
			if msg == nil {
				return nil
			}
			role := jsonl.String(msg["role"])
			ts := jsonl.TimeOf(msg["timestamp"])
			if ts.IsZero() {
				ts = jsonl.TimeOf(m["timestamp"])
			}
			if ts.IsZero() {
				ts = time.Now()
			}
			switch role {
			case "user":
				ev := injects.ClassifyUser(jsonl.TextContent(msg["content"]), jsonl.ImageURLs(msg["content"]))
				ev.Timestamp = ts
				ev.SourceKind = "pi.user"
				if textutil.NonEmpty(ev.Text) || len(ev.Images) > 0 || len(ev.Injects) > 0 {
					sess.Events = append(sess.Events, ev)
				}
			case "assistant":
				text, thinking, tools := parseAssistant(msg["content"])
				ev := ir.Event{
					Timestamp: ts, Role: ir.RoleAssistant, Text: text,
					Thinking: thinking, Tools: tools, SourceKind: "pi.assistant",
					Interrupted: jsonl.GetBool(msg, "interrupted") || jsonl.GetBool(m, "interrupted"),
				}
				sess.Events = append(sess.Events, ev)
				lastAsst = len(sess.Events) - 1
			case "toolResult":
				callID := jsonl.String(msg["toolCallId"])
				name := jsonl.String(msg["toolName"])
				result := jsonl.TextContent(msg["content"])
				isErr := jsonl.GetBool(msg, "isError")
				if lastAsst >= 0 {
					attachResult(&sess.Events[lastAsst], callID, name, result, isErr)
				}
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
	if opt.DryRun {
		return paths.PiDir(outCWD), nil
	}
	id := uuid.New().String()
	now := time.Now().UTC()
	fname := now.Format("2006-01-02T15-04-05.000Z") + "_" + id + ".jsonl"
	path := filepath.Join(paths.PiDir(outCWD), fname)
	leafID := func() string { return uuid.New().String() }
	sessionID := leafID()
	parent := sessionID
	objs := []any{
		map[string]any{
			"type": "session", "id": sessionID, "parentId": nil, "timestamp": now.UnixMilli(),
			"cwd": outCWD, "version": 3,
		},
		map[string]any{
			"type": "session_info", "id": leafID(), "parentId": parent, "timestamp": now.UnixMilli(),
			"name": ir.DisplayTitle(sess, opt),
		},
	}
	parent = objs[1].(map[string]any)["id"].(string)
	bannerID := leafID()
	objs = append(objs, map[string]any{
		"type": "message", "id": bannerID, "parentId": parent, "timestamp": now.UnixMilli(),
		"message": map[string]any{
			"role": "user", "timestamp": now.UnixMilli(),
			"content": []any{map[string]any{"type": "text", "text": ir.ImportBanner(sess)}},
		},
	})
	parent = bannerID
	ackID := leafID()
	objs = append(objs, map[string]any{
		"type": "message", "id": ackID, "parentId": parent, "timestamp": now.UnixMilli(),
		"message": map[string]any{
			"role": "assistant", "timestamp": now.UnixMilli(),
			"content": []any{map[string]any{"type": "text", "text": "Imported. Ready to continue from the transferred memory."}},
		},
	})
	parent = ackID
	for _, ev := range sess.Events {
		text := ir.PrefixRole(ev, ir.Narrate(ev, opt))
		if !textutil.NonEmpty(text) && !textutil.NonEmpty(ev.Thinking) {
			continue
		}
		role := "user"
		content := []any{}
		if ev.Role == ir.RoleAssistant {
			role = "assistant"
			if textutil.NonEmpty(ev.Thinking) {
				content = append(content, map[string]any{"type": "thinking", "thinking": ev.Thinking})
			}
		}
		if textutil.NonEmpty(text) {
			content = append(content, map[string]any{"type": "text", "text": text})
		}
		if len(content) == 0 {
			continue
		}
		mid := leafID()
		objs = append(objs, map[string]any{
			"type": "message", "id": mid, "parentId": parent, "timestamp": ev.Timestamp.UnixMilli(),
			"message": map[string]any{
				"role": role, "timestamp": ev.Timestamp.UnixMilli(), "content": content,
			},
		})
		parent = mid
	}
	if err := jsonl.WriteLines(path, objs); err != nil {
		return "", err
	}
	return path, nil
}

func parsePiID(name string) string {
	base := strings.TrimSuffix(name, ".jsonl")
	if i := strings.LastIndex(base, "_"); i >= 0 {
		return base[i+1:]
	}
	return base
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
		case "toolCall":
			args := jsonl.CompactJSON(m["arguments"], 8000)
			if args == "" {
				args = jsonl.String(m["arguments"])
			}
			tools = append(tools, ir.ToolTrace{
				CallID: jsonl.String(m["id"]), Name: jsonl.String(m["name"]), Arguments: args,
			})
		}
	}
	text = strings.Join(texts, "\n")
	return
}

func attachResult(ev *ir.Event, callID, name, result string, isErr bool) {
	for i := range ev.Tools {
		if ev.Tools[i].CallID == callID || (callID == "" && ev.Tools[i].Result == "") {
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
