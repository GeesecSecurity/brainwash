package codex

import (
	"bytes"
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

func (Parser) Name() ir.Slot { return ir.SlotCodex }
func (Parser) Label() string { return "Codex" }

func (Parser) List(cwd string) ([]ir.SessionRef, error) {
	root := paths.CodexSessionsRoot()
	want := ""
	if strings.TrimSpace(cwd) != "" {
		want = filepath.Clean(cwd)
	}
	var out []ir.SessionRef
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasPrefix(d.Name(), "rollout-") || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		obj, err := jsonl.FirstObject(p)
		if err != nil {
			return nil
		}
		payload := obj
		if jsonl.String(obj["type"]) == "session_meta" {
			if inner := jsonl.GetMap(obj, "payload"); inner != nil {
				payload = inner
			}
		}
		scwd := jsonl.String(payload["cwd"])
		if want != "" && scwd != "" && filepath.Clean(scwd) != want {
			return nil
		}
		id := jsonl.String(payload["id"])
		if id == "" {
			id = strings.TrimSuffix(strings.TrimPrefix(d.Name(), "rollout-"), ".jsonl")
		}
		title := jsonl.String(payload["name"])
		if title == "" {
			for _, row := range jsonl.PeekObjects(p, 50, 256*1024) {
				if jsonl.String(row["type"]) != "response_item" {
					continue
				}
				item := jsonl.GetMap(row, "payload")
				if jsonl.String(item["type"]) != "message" {
					continue
				}
				if jsonl.String(item["role"]) != "user" {
					continue
				}
				title = injects.CleanTitle(jsonl.TextContent(item["content"]))
				if title != "" {
					break
				}
			}
		}
		if title == "" {
			title = id
		}
		created := jsonl.TimeOf(payload["timestamp"])
		if created.IsZero() {
			created = jsonl.FileMod(p)
		}
		out = append(out, ir.SessionRef{
			Slot: ir.SlotCodex, ID: id, CWD: scwd, Title: title, Path: p,
			CreatedAt: created, UpdatedAt: jsonl.FileMod(p), Bytes: jsonl.FileSize(p),
		})
		return nil
	})
	return out, nil
}

func (Parser) Load(ref ir.SessionRef) (*ir.Session, error) {
	sess := &ir.Session{
		ID: ref.ID, Slot: ir.SlotCodex, CWD: ref.CWD, Title: ref.Title,
		SourcePath: ref.Path, CreatedAt: ref.CreatedAt, UpdatedAt: ref.UpdatedAt,
		Notes: []string{"Imported from Codex session " + ref.ID},
	}
	lastAsst := -1
	err := jsonl.ReadLines(ref.Path, func(b []byte) error {
		var m map[string]any
		if json.Unmarshal(b, &m) != nil {
			return nil
		}
		typ := jsonl.String(m["type"])
		payload := jsonl.GetMap(m, "payload")
		if payload == nil {
			payload = m
		}
		ts := jsonl.TimeOf(m["timestamp"])
		if ts.IsZero() {
			ts = jsonl.TimeOf(payload["timestamp"])
		}
		if ts.IsZero() {
			ts = time.Now()
		}
		switch typ {
		case "session_meta":
			if c := jsonl.String(payload["cwd"]); c != "" {
				sess.CWD = c
			}
			if n := jsonl.String(payload["name"]); n != "" {
				sess.Title = n
			}
			if id := jsonl.String(payload["id"]); id != "" {
				sess.ID = id
			}
			if t := jsonl.TimeOf(payload["timestamp"]); !t.IsZero() {
				sess.CreatedAt = t
			}
		case "event_msg":
			// skip noisy runtime
		case "response_item":
			item := payload
			if inner := jsonl.GetMap(payload, "payload"); inner != nil && inner["type"] != nil {
				item = inner
			}
			handleItem(sess, item, ts, &lastAsst)
		case "turn_item", "item":
			handleItem(sess, payload, ts, &lastAsst)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	clean := injects.CleanTitle(sess.Title)
	if clean != "" {
		sess.Title = clean
	} else if sess.Title == "" || sess.Title == sess.ID {
		sess.Title = ir.FirstUserTitle(sess.Events, sess.ID)
	}
	return sess, nil
}

func handleItem(sess *ir.Session, item map[string]any, ts time.Time, lastAsst *int) {
	typ := jsonl.String(item["type"])
	role := jsonl.String(item["role"])
	switch typ {
	case "message":
		content := item["content"]
		text := jsonl.TextContent(content)
		imgs := jsonl.ImageURLs(content)
		if role == "user" || role == "" && looksUser(item) {
			ev := injects.ClassifyUser(text, imgs)
			ev.Timestamp = ts
			ev.SourceKind = "codex.user"
			if textutil.NonEmpty(ev.Text) || len(ev.Images) > 0 || len(ev.Injects) > 0 {
				sess.Events = append(sess.Events, ev)
			}
			return
		}
		if role == "assistant" || role == "system" {
			if role == "system" {
				ev := injects.ClassifyUser(text, imgs)
				ev.Timestamp = ts
				ev.Role = ir.RoleSystem
				ev.Kind = ir.KindInject
				ev.SourceKind = "codex.system"
				if textutil.NonEmpty(ev.Text) || len(ev.Injects) > 0 {
					sess.Events = append(sess.Events, ev)
				}
				return
			}
			sess.Events = append(sess.Events, ir.Event{
				Timestamp: ts, Role: ir.RoleAssistant, Text: text, Images: imgs,
				Interrupted: jsonl.GetBool(item, "interrupted"),
				SourceKind: "codex.assistant",
			})
			*lastAsst = len(sess.Events) - 1
		}
	case "function_call", "custom_tool_call", "computer_call":
		name := jsonl.String(item["name"])
		if name == "" {
			name = typ
		}
		args := jsonl.String(item["arguments"])
		if args == "" {
			args = jsonl.String(item["input"])
		}
		if args == "" {
			args = jsonl.CompactJSON(item["arguments"], 8000)
		}
		tr := ir.ToolTrace{CallID: jsonl.String(item["call_id"]), Name: name, Arguments: args}
		if *lastAsst >= 0 {
			sess.Events[*lastAsst].Tools = append(sess.Events[*lastAsst].Tools, tr)
		} else {
			sess.Events = append(sess.Events, ir.Event{
				Timestamp: ts, Role: ir.RoleAssistant, Tools: []ir.ToolTrace{tr}, SourceKind: "codex.tool",
			})
			*lastAsst = len(sess.Events) - 1
		}
	case "function_call_output", "custom_tool_call_output", "computer_call_output":
		callID := jsonl.String(item["call_id"])
		out := jsonl.TextContent(item["output"])
		if out == "" {
			out = jsonl.String(item["output"])
		}
		if *lastAsst >= 0 {
			attach(&sess.Events[*lastAsst], callID, "", out, false)
		}
	case "reasoning":
		sum := jsonl.TextContent(item["summary"])
		if sum == "" {
			sum = jsonl.TextContent(item["content"])
		}
		if textutil.NonEmpty(sum) {
			if *lastAsst >= 0 && sess.Events[*lastAsst].Thinking == "" {
				sess.Events[*lastAsst].Thinking = sum
			}
		}
	}
}

func looksUser(item map[string]any) bool {
	return jsonl.String(item["role"]) == "user"
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

func (Parser) Write(sess *ir.Session, outCWD string, opt ir.WriteOptions) (string, error) {
	now := time.Now().UTC()
	id := uuid.Must(uuid.NewV7()).String()
	day := now.Format("2006/01/02")
	fname := "rollout-" + now.Format("2006-01-02T15-04-05") + "-" + id + ".jsonl"
	path := filepath.Join(paths.CodexSessionsRoot(), day, fname)
	if opt.DryRun {
		return path, nil
	}
	title := ir.DisplayTitle(sess, opt)
	ts := rfc3339Milli(now)
	events := make([]ir.Event, 0, len(sess.Events)+2)
	events = append(events,
		ir.Event{Timestamp: now, Role: ir.RoleUser, Text: ir.ImportBanner(sess)},
		ir.Event{Timestamp: now, Role: ir.RoleAssistant, Text: "Imported. Ready to continue from the transferred memory."},
	)
	for _, ev := range sess.Events {
		text := ir.PrefixRole(ev, ir.Narrate(ev, opt))
		if !textutil.NonEmpty(text) && ev.Thinking == "" {
			continue
		}
		role := ir.RoleUser
		if ev.Role == ir.RoleAssistant {
			role = ir.RoleAssistant
		}
		events = append(events, ir.Event{Timestamp: ev.Timestamp, Role: role, Text: text, Thinking: ev.Thinking})
	}
	objs := rolloutFromEvents(id, outCWD, ts, events)
	if err := jsonl.WriteLines(path, objs); err != nil {
		return "", err
	}
	preview := firstUserPreview(sess)
	if preview == "" {
		preview = title
	}
	_ = jsonl.AppendLine(paths.CodexSessionIndex(), map[string]any{
		"id": id, "thread_name": title, "updated_at": ts,
	})
	if err := registerDesktopThread(id, path, outCWD, title, now, now, preview); err != nil {
		return path, err
	}
	return path, nil
}

func rfc3339Milli(t time.Time) string {
	return t.UTC().Format("2006-01-02T15:04:05.000Z")
}

type asstBuf struct {
	text, thinking string
	ts             time.Time
}

func rolloutFromEvents(threadID, cwd, metaTS string, events []ir.Event) []any {
	objs := []any{[]byte(sessionMetaLine(threadID, cwd, metaTS))}
	var user string
	var userTS time.Time
	var asst []asstBuf
	flush := func() {
		if user == "" && len(asst) == 0 {
			return
		}
		t := userTS
		if t.IsZero() {
			t = time.Now().UTC()
		}
		turnID := uuid.Must(uuid.NewV7()).String()
		started := t.Unix()
		objs = append(objs, eventMsg(t, map[string]any{
			"type": "task_started", "turn_id": turnID, "started_at": started,
			"model_context_window": 258400, "collaboration_mode_kind": "default",
		}))
		if user != "" {
			objs = append(objs, responseMessage("user", user, t))
			objs = append(objs, eventMsg(t, map[string]any{
				"type": "user_message", "message": user,
				"images": []any{}, "local_images": []any{}, "audio": []any{}, "local_audio": []any{}, "text_elements": []any{},
			}))
		}
		last := ""
		completed := t
		for _, a := range asst {
			at := a.ts
			if at.IsZero() {
				at = t
			}
			if textutil.NonEmpty(a.thinking) {
				objs = append(objs, eventMsg(at, map[string]any{"type": "agent_reasoning", "text": a.thinking}))
			}
			if textutil.NonEmpty(a.text) {
				objs = append(objs, responseMessage("assistant", a.text, at))
				objs = append(objs, eventMsg(at, map[string]any{"type": "agent_message", "message": a.text}))
				last = a.text
				completed = at
			}
		}
		end := map[string]any{
			"type": "task_complete", "turn_id": turnID, "started_at": started, "completed_at": completed.Unix(),
		}
		if last != "" {
			end["last_agent_message"] = last
		}
		objs = append(objs, eventMsg(completed, end))
		user, asst, userTS = "", nil, time.Time{}
	}
	for _, ev := range events {
		ts := ev.Timestamp
		if ts.IsZero() {
			ts = time.Now().UTC()
		}
		switch ev.Role {
		case ir.RoleUser:
			if user != "" || len(asst) > 0 {
				flush()
			}
			user, userTS = ev.Text, ts
		default:
			asst = append(asst, asstBuf{text: ev.Text, thinking: ev.Thinking, ts: ts})
		}
	}
	flush()
	return objs
}

func responseMessage(role, text string, t time.Time) map[string]any {
	ctype := "input_text"
	if role == "assistant" {
		ctype = "output_text"
	}
	return map[string]any{
		"timestamp": rfc3339Milli(t.UTC()),
		"type":      "response_item",
		"payload": map[string]any{
			"type": "message", "id": "msg_" + uuid.Must(uuid.NewV7()).String(), "role": role,
			"content": []any{map[string]any{"type": ctype, "text": text}},
		},
	}
}

func eventMsg(t time.Time, payload map[string]any) map[string]any {
	return map[string]any{
		"timestamp": rfc3339Milli(t.UTC()),
		"type":      "event_msg",
		"payload":   payload,
	}
}

func sessionMetaLine(id, cwd, ts string) string {
	// Native Codex serde requires originator + cli_version on SessionMeta. Extra keys are ignored.
	return `{"timestamp":` + quoteJSON(ts) + `,"type":"session_meta","payload":{` +
		`"session_id":` + quoteJSON(id) +
		`,"id":` + quoteJSON(id) +
		`,"timestamp":` + quoteJSON(ts) +
		`,"cwd":` + quoteJSON(cwd) +
		`,"originator":"Codex Desktop"` +
		`,"cli_version":` + quoteJSON(desktopCLIVersion) +
		`,"source":"vscode"` +
		`,"thread_source":"user"` +
		`,"model_provider":"man"` +
		`,"history_mode":"legacy"}}` + "\n"
}

func quoteJSON(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func RepairDesktopHistory(path string) error {
	if err := RepairSessionMeta(path); err != nil {
		return err
	}
	var lines [][]byte
	hasUserEvt := false
	hasAsstEvt := false
	err := jsonl.ReadLines(path, func(b []byte) error {
		cp := append([]byte(nil), b...)
		lines = append(lines, cp)
		var m map[string]any
		if json.Unmarshal(cp, &m) != nil {
			return nil
		}
		if jsonl.String(m["type"]) != "event_msg" {
			return nil
		}
		pl := jsonl.GetMap(m, "payload")
		switch jsonl.String(pl["type"]) {
		case "user_message":
			hasUserEvt = true
		case "agent_message":
			hasAsstEvt = true
		}
		return nil
	})
	if err != nil || (hasUserEvt && hasAsstEvt) {
		return err
	}
	var events []ir.Event
	for _, line := range lines {
		var m map[string]any
		if json.Unmarshal(line, &m) != nil {
			continue
		}
		if jsonl.String(m["type"]) != "response_item" {
			continue
		}
		item := jsonl.GetMap(m, "payload")
		if jsonl.String(item["type"]) != "message" {
			continue
		}
		role := jsonl.String(item["role"])
		text := jsonl.TextContent(item["content"])
		if !textutil.NonEmpty(text) {
			continue
		}
		ts := jsonl.TimeOf(m["timestamp"])
		switch role {
		case "user":
			events = append(events, ir.Event{Timestamp: ts, Role: ir.RoleUser, Text: text})
		case "assistant":
			events = append(events, ir.Event{Timestamp: ts, Role: ir.RoleAssistant, Text: text})
		}
	}
	if len(events) == 0 {
		return nil
	}
	id, cwd, _, created := peekMeta(path)
	if id == "" {
		return nil
	}
	ts := rfc3339Milli(created)
	if created.IsZero() {
		ts = rfc3339Milli(time.Now())
	}
	return jsonl.WriteLines(path, rolloutFromEvents(id, cwd, ts, events))
}

func RepairSessionMeta(path string) error {
	first, rest, err := splitFirstLine(path)
	if err != nil {
		return err
	}
	var obj map[string]any
	if json.Unmarshal(first, &obj) != nil {
		return nil
	}
	payload := jsonl.GetMap(obj, "payload")
	if payload == nil {
		payload = obj
	}
	id := jsonl.String(payload["id"])
	if id == "" {
		id = jsonl.String(payload["session_id"])
	}
	cwd := jsonl.String(payload["cwd"])
	ts := jsonl.String(obj["timestamp"])
	if ts == "" {
		ts = jsonl.String(payload["timestamp"])
	}
	if ts == "" {
		ts = rfc3339Milli(time.Now())
	}
	if jsonl.String(payload["originator"]) != "" && jsonl.String(payload["cli_version"]) != "" && jsonl.String(payload["session_id"]) != "" {
		return nil
	}
	if id == "" {
		return nil
	}
	line := []byte(sessionMetaLine(id, cwd, ts))
	out := append(line, rest...)
	return os.WriteFile(path, out, 0o644)
}

func splitFirstLine(path string) (first, rest []byte, err error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	i := bytes.IndexByte(raw, '\n')
	if i < 0 {
		return raw, nil, nil
	}
	return raw[:i], raw[i+1:], nil
}
