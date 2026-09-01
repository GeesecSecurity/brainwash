package injects

import (
	"strings"
	"testing"
)

func TestSplitUnclosedSkill(t *testing.T) {
	raw := "帮我看下登录超时\n<skill name=\"auth\">\npartial xml without close"
	s := Split(raw)
	if s.Text != "帮我看下登录超时" {
		t.Fatalf("text=%q", s.Text)
	}
	if len(s.Injects) != 1 || s.Injects[0].Kind != "skill" {
		t.Fatalf("injects=%+v", s.Injects)
	}
}

func TestCleanTitleStripsEnv(t *testing.T) {
	raw := "<environment_context>\ncwd: /tmp\n</environment_context>\nfix login timeout please"
	got := CleanTitle(raw)
	if got != "fix login timeout please" {
		t.Fatalf("title=%q", got)
	}
}

func TestClassifyInjectOnly(t *testing.T) {
	ev := ClassifyUser("<system-reminder>do not mention</system-reminder>", nil)
	if ev.Kind != "inject" {
		t.Fatalf("kind=%s text=%s", ev.Kind, ev.Text)
	}
}

func TestExtractImagesStripsMarkdown(t *testing.T) {
	text, imgs := ExtractImages("看下这个\n![image](data:image/png;base64,abc)\n帮我", nil)
	if strings.Contains(text, "![image]") {
		t.Fatalf("text still has markdown: %q", text)
	}
	if len(imgs) != 1 || !strings.HasPrefix(imgs[0], "data:image") {
		t.Fatalf("imgs=%v", imgs)
	}
}

func TestExtractImagesStripsPlaceholder(t *testing.T) {
	text, imgs := ExtractImages("无法安装客户端\n<image>", []string{"data:image/png;base64,abc"})
	if strings.Contains(strings.ToLower(text), "image") {
		t.Fatalf("text still has image tag: %q", text)
	}
	if len(imgs) != 1 {
		t.Fatalf("imgs=%v", imgs)
	}
}
