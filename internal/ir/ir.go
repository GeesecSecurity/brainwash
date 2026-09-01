package ir

import "time"

// Slot is a registered agent memory format.
type Slot string

const (
	SlotPi     Slot = "pi"
	SlotCodex  Slot = "codex"
	SlotClaude Slot = "claude"
	SlotDSH    Slot = "dsh"
)

func (s Slot) Label() string {
	switch s {
	case SlotPi:
		return "pi"
	case SlotCodex:
		return "Codex"
	case SlotClaude:
		return "Claude Code"
	case SlotDSH:
		return "DeepSeek Harness"
	default:
		return string(s)
	}
}

type SessionRef struct {
	Slot      Slot      `json:"slot"`
	ID        string    `json:"id"`
	CWD       string    `json:"cwd"`
	Title     string    `json:"title"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Bytes     int64     `json:"bytes,omitempty"`
}

type ToolTrace struct {
	CallID    string `json:"callId,omitempty"`
	Name      string `json:"name"`
	Arguments string `json:"arguments,omitempty"`
	Result    string `json:"result,omitempty"`
	IsError   bool   `json:"isError,omitempty"`
}

const (
	RoleUser      = "user"
	RoleAssistant = "assistant"
	RoleSummary   = "summary"
	RoleSystem    = "system"

	KindInput       = "input"
	KindInject      = "inject"
	KindCompaction  = "compaction"
	KindHandoff     = "handoff"
)

type Inject struct {
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
	Text    string `json:"text,omitempty"`
}

type Event struct {
	Timestamp  time.Time   `json:"timestamp"`
	Role       string      `json:"role"`
	Kind       string      `json:"kind,omitempty"`
	Text       string      `json:"text"`
	Thinking   string      `json:"thinking,omitempty"`
	Tools      []ToolTrace `json:"tools,omitempty"`
	Images     []string    `json:"images,omitempty"`
	Injects    []Inject    `json:"injects,omitempty"`
	Interrupted bool      `json:"interrupted,omitempty"`
	SourceKind string      `json:"sourceKind,omitempty"`
}

type Session struct {
	ID         string    `json:"id"`
	Slot       Slot      `json:"slot"`
	CWD        string    `json:"cwd"`
	Title      string    `json:"title"`
	SourcePath string    `json:"sourcePath"`
	CreatedAt  time.Time `json:"createdAt"`
	UpdatedAt  time.Time `json:"updatedAt"`
	Events     []Event   `json:"events"`
	Notes      []string  `json:"notes,omitempty"`
}

type WriteOptions struct {
	IncludeTools bool   `json:"includeTools"`
	MaxToolChars int    `json:"maxToolChars"`
	NamePrefix   string `json:"namePrefix"`
	DryRun       bool   `json:"dryRun"`
}

func (o WriteOptions) ToolLimit() int {
	if o.MaxToolChars <= 0 {
		return 4000
	}
	return o.MaxToolChars
}

func DisplayTitle(sess *Session, opt WriteOptions) string {
	prefix := opt.NamePrefix
	if prefix == "" {
		prefix = "[imported " + sess.Slot.Label() + "] "
	}
	return prefix + sess.Title
}

func ImportBanner(sess *Session) string {
	return "[memory-transfer] Imported conversation from " + sess.Slot.Label() + "\n" +
		"source-id: " + sess.ID + "\n" +
		"source-path: " + sess.SourcePath + "\n" +
		"original-cwd: " + sess.CWD + "\n" +
		"title: " + sess.Title + "\n\n" +
		"Historical tool invocations were converted into narrative text so they cannot collide with this agent's live tool schema.\n" +
		"Continue from the imported progress."
}

func Narrate(ev Event, opt WriteOptions) string {
	text := ev.Text
	if !opt.IncludeTools || len(ev.Tools) == 0 {
		return text
	}
	body := RenderTools(ev.Tools, opt.ToolLimit())
	if text == "" {
		return body
	}
	return text + "\n\n" + body
}

func RenderTools(tools []ToolTrace, limit int) string {
	out := "[Imported historical actions from another coding agent.\nThese are NOT live tool calls. Do not try to re-invoke them with this agent's tools.]\n"
	for i, t := range tools {
		out += "\n### Action " + itoa(i+1) + ": " + t.Name + "\n"
		if t.CallID != "" {
			out += "- id: " + t.CallID + "\n"
		}
		if t.Arguments != "" {
			out += "- arguments:\n```\n" + Truncate(t.Arguments, limit) + "\n```\n"
		}
		if t.Result != "" {
			tag := ""
			if t.IsError {
				tag = " (error)"
			}
			out += "- result" + tag + ":\n```\n" + Truncate(t.Result, limit) + "\n```\n"
		}
	}
	return out
}

func PrefixRole(ev Event, text string) string {
	switch ev.Role {
	case RoleSummary:
		return "[Imported summary]\n" + text
	case RoleSystem:
		if ev.Kind != "" {
			return "[Imported " + ev.Kind + "]\n" + text
		}
		return "[Imported system note]\n" + text
	default:
		return text
	}
}

func Truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "\n…(truncated)…"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}

func FirstUserTitle(events []Event, fallback string) string {
	for _, ev := range events {
		if ev.Role != RoleUser || ev.Kind == KindInject {
			continue
		}
		t := compactWS(ev.Text)
		if t == "" {
			continue
		}
		if len(t) > 80 {
			t = t[:80]
		}
		return t
	}
	return fallback
}

func compactWS(s string) string {
	out := make([]rune, 0, len(s))
	space := false
	for _, r := range s {
		if r == '\n' || r == '\r' || r == '\t' || r == ' ' {
			if !space && len(out) > 0 {
				out = append(out, ' ')
				space = true
			}
			continue
		}
		space = false
		out = append(out, r)
	}
	return string(out)
}
