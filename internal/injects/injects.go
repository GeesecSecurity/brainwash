package injects

import (
	"regexp"
	"strings"
	"unicode/utf8"

	"brainwash/internal/ir"
)

type Hit struct {
	Kind    string `json:"kind"`
	Summary string `json:"summary"`
	Text    string `json:"text,omitempty"`
}

type SplitResult struct {
	Text    string
	Injects []Hit
}

var summaries = map[string]string{
	"skill":           "🧩 技能调用",
	"instruction":     "📌 系统指令",
	"system_reminder": "📌 系统提醒",
	"ctx_hint":        "🔎 相关记忆",
	"ctx_note":        "📝 上下文笔记",
	"environment":     "🖥️ 环境上下文",
	"agents_md":       "📋 AGENTS.md",
	"handoff":         "🔁 会话交接",
	"compaction":      "🔁 上下文压缩",
	"heartbeat":       "⏱️ 心跳",
	"review":          "🔎 评估上下文",
	"sys_prompt":      "📋 系统提示",
	"user_profile":    "👤 用户资料",
	"session_history": "🗂️ 会话历史",
	"compartment":     "📦 记忆隔间",
}

func reTag(name string) *regexp.Regexp {
	return regexp.MustCompile(`(?is)<` + name + `\b[^>]*>.*?</` + name + `\s*>`)
}

var closedTags = []struct {
	kind string
	re   *regexp.Regexp
}{
	{"instruction", reTag("instruction")},
	{"system_reminder", reTag("system-reminder")},
	{"ctx_hint", reTag("ctx-search-hint")},
	{"ctx_note", reTag(`ctx[_-]note`)},
	{"skill", reTag("skill")},
	{"environment", reTag("environment_context")},
	{"agents_md", reTag("INSTRUCTIONS")},
	{"user_profile", reTag("user-profile")},
	{"session_history", reTag(`session-history(?:-since)?`)},
	{"compartment", reTag(`(?:new-)?compartments?`)},
}

var (
	openSkill     = regexp.MustCompile(`(?is)<skill\b[^>]*>`)
	openEnv       = regexp.MustCompile(`(?is)<environment_context\b[^>]*>`)
	openInstr     = regexp.MustCompile(`(?is)<instruction\b[^>]*>`)
	openReminder  = regexp.MustCompile(`(?is)<system-reminder\b[^>]*>`)
	openAgents    = regexp.MustCompile(`(?is)<INSTRUCTIONS\b[^>]*>`)
	openProfile   = regexp.MustCompile(`(?is)<user-profile\b[^>]*>`)
	openHistory   = regexp.MustCompile(`(?is)<session-history\b[^>]*>`)
	myRequestMark = regexp.MustCompile(`(?im)(?:^|\n)\s{0,3}#{0,6}\s*my request for codex\s*:?\s*`)
)

func Split(text string) SplitResult {
	rest := text
	var hits []Hit
	for _, spec := range closedTags {
		for _, m := range spec.re.FindAllString(rest, -1) {
			hits = append(hits, Hit{Kind: spec.kind, Summary: summaries[spec.kind], Text: clip(m, 4000)})
		}
		rest = spec.re.ReplaceAllString(rest, "\n")
	}
	if idx, kind := earliestOpen(rest); idx >= 0 {
		hits = append(hits, Hit{Kind: kind, Summary: summaries[kind], Text: clip(rest[idx:], 4000)})
		rest = rest[:idx]
	}
	if loc := myRequestMark.FindStringIndex(rest); loc != nil {
		head := strings.TrimSpace(rest[:loc[0]])
		if head != "" {
			hits = append(hits, Hit{Kind: "instruction", Summary: summaries["instruction"], Text: clip(head, 4000)})
		}
		rest = rest[loc[1]:]
	}
	rest = collapseNL(strings.TrimSpace(rest))
	if extra, ok := detectWhole(rest); ok {
		hits = append(hits, extra)
		rest = ""
	}
	return SplitResult{Text: rest, Injects: hits}
}

func earliestOpen(s string) (int, string) {
	type cand struct {
		kind string
		re   *regexp.Regexp
	}
	best, kind := -1, ""
	for _, c := range []cand{
		{"skill", openSkill},
		{"environment", openEnv},
		{"instruction", openInstr},
		{"system_reminder", openReminder},
		{"agents_md", openAgents},
		{"user_profile", openProfile},
		{"session_history", openHistory},
	} {
		loc := c.re.FindStringIndex(s)
		if loc == nil {
			continue
		}
		if best < 0 || loc[0] < best {
			best, kind = loc[0], c.kind
		}
	}
	return best, kind
}

func detectWhole(text string) (Hit, bool) {
	t := strings.TrimSpace(text)
	if t == "" {
		return Hit{}, false
	}
	head := t
	if len(head) > 300 {
		head = head[:300]
	}
	switch {
	case strings.HasPrefix(t, "# AGENTS.md") || strings.Contains(head, "<INSTRUCTIONS>"):
		return Hit{"agents_md", summaries["agents_md"], clip(t, 4000)}, true
	case strings.Contains(t, "<environment_context>"):
		return Hit{"environment", summaries["environment"], clip(t, 4000)}, true
	case strings.HasPrefix(t, "Another language model started to solve this problem"):
		return Hit{"handoff", summaries["handoff"], clip(t, 4000)}, true
	case strings.Contains(t, "CONTEXT CHECKPOINT COMPACTION") || strings.Contains(t, "Create a handoff summary"):
		return Hit{"compaction", summaries["compaction"], clip(t, 4000)}, true
	case strings.HasPrefix(t, "<heartbeat"):
		return Hit{"heartbeat", summaries["heartbeat"], clip(t, 4000)}, true
	case strings.HasPrefix(t, "The following is the Codex agent history") || strings.Contains(t, ">>> TRANSCRIPT") || strings.Contains(t, ">>> APPROVAL REQUEST"):
		return Hit{"review", summaries["review"], clip(t, 4000)}, true
	case strings.HasPrefix(t, "You are a helpful assistant") || strings.HasPrefix(t, "You are Codex") || strings.HasPrefix(t, "You are an expert coding assistant"):
		return Hit{"sys_prompt", summaries["sys_prompt"], clip(t, 4000)}, true
	}
	return Hit{}, false
}

func ClassifyUser(raw string, images []string) ir.Event {
	s := Split(raw)
	text, imgs := ExtractImages(s.Text, images)
	ev := ir.Event{Role: ir.RoleUser, Images: imgs}
	for _, h := range s.Injects {
		ev.Injects = append(ev.Injects, ir.Inject{Kind: h.Kind, Summary: h.Summary, Text: h.Text})
	}
	if strings.TrimSpace(text) == "" && len(s.Injects) > 0 {
		ev.Kind = ir.KindInject
		ev.Text = s.Injects[0].Summary
		if len(s.Injects) > 1 {
			var parts []string
			for _, h := range s.Injects {
				parts = append(parts, h.Summary)
			}
			ev.Text = strings.Join(parts, " · ")
		}
		return ev
	}
	ev.Kind = ir.KindInput
	ev.Text = text
	return ev
}

func PrepareEvent(ev *ir.Event) {
	if ev == nil {
		return
	}
	text, imgs := ExtractImages(ev.Text, ev.Images)
	ev.Text = text
	ev.Images = imgs
}

var (
	mdImage          = regexp.MustCompile(`!\[[^\]]*\]\(([^)]*)\)`)
	htmlImage        = regexp.MustCompile(`(?i)<img\b[^>]*\bsrc=['"]([^'"]+)['"][^>]*>`)
	imagePlaceholder = regexp.MustCompile(`(?i)\s*(?:<image\s*/?>|</image>|\[image\]|\(image\)|!\[image\])\s*`)
)

func ExtractImages(text string, extra []string) (string, []string) {
	var images []string
	seen := map[string]bool{}
	push := func(u string) {
		u = strings.TrimSpace(u)
		if u == "" || seen[u] {
			return
		}
		seen[u] = true
		images = append(images, u)
	}
	for _, u := range extra {
		push(u)
	}
	rest := mdImage.ReplaceAllStringFunc(text, func(m string) string {
		sub := mdImage.FindStringSubmatch(m)
		if len(sub) > 1 {
			push(sub[1])
		}
		return ""
	})
	rest = htmlImage.ReplaceAllStringFunc(rest, func(m string) string {
		sub := htmlImage.FindStringSubmatch(m)
		if len(sub) > 1 {
			push(sub[1])
		}
		return ""
	})
	rest = imagePlaceholder.ReplaceAllString(rest, "")
	rest = collapseNL(rest)
	switch strings.ToLower(strings.TrimSpace(rest)) {
	case "image", "[image]", "(image)", "<image>":
		rest = ""
	}
	return rest, images
}

func CleanTitle(text string) string {
	s := Split(text)
	skipUntilRequest := false
	for _, line := range strings.Split(s.Text, "\n") {
		trim := strings.TrimSpace(line)
		low := strings.ToLower(strings.TrimLeft(trim, "# "))
		switch {
		case low == "files mentioned by the user:" || strings.HasPrefix(low, "files mentioned by the user"):
			skipUntilRequest = true
			continue
		case low == "chrome tabs:" || strings.HasPrefix(low, "chrome tabs:"):
			skipUntilRequest = true
			continue
		case strings.HasPrefix(low, "my request:"):
			skipUntilRequest = false
			trim = strings.TrimSpace(trim[strings.Index(strings.ToLower(trim), "my request:")+len("my request:"):])
			if trim == "" {
				continue
			}
		case skipUntilRequest:
			continue
		case trim == "":
			continue
		}
		trim = strings.TrimLeft(trim, "# ")
		if utf8.RuneCountInString(trim) > 72 {
			trim = string([]rune(trim)[:72]) + "…"
		}
		return trim
	}
	return ""
}

func clip(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n] + "\n…(truncated)…"
}

func collapseNL(s string) string {
	s = strings.ReplaceAll(s, "\r\n", "\n")
	for strings.Contains(s, "\n\n\n") {
		s = strings.ReplaceAll(s, "\n\n\n", "\n\n")
	}
	return strings.TrimSpace(s)
}
