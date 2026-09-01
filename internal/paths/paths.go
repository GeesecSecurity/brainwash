package paths

import (
	"os"
	"path/filepath"
	"strings"
)

func Home() string {
	if h := os.Getenv("HOME"); h != "" {
		return h
	}
	h, err := os.UserHomeDir()
	if err != nil {
		return "/"
	}
	return h
}

func ResolveCWD(raw string) string {
	if strings.TrimSpace(raw) == "" {
		wd, _ := os.Getwd()
		raw = wd
	}
	abs, err := filepath.Abs(raw)
	if err != nil {
		return raw
	}
	return filepath.Clean(abs)
}

func PiSessionsRoot() string {
	return filepath.Join(Home(), ".pi", "agent", "sessions")
}

func CodexSessionsRoot() string {
	return filepath.Join(Home(), ".codex", "sessions")
}

func ClaudeProjectsRoot() string {
	return filepath.Join(Home(), ".claude", "projects")
}

func DSHSessionsRoot() string {
	return filepath.Join(Home(), ".dsh", "sessions")
}

func DSHWorkspace() string {
	return filepath.Join(Home(), ".dsh", "storages", "workspace.json")
}

func DSHProjCache() string {
	return filepath.Join(Home(), ".dsh", "storages", "session_projcache.json")
}

func CodexSessionIndex() string {
	return filepath.Join(Home(), ".codex", "session_index.jsonl")
}

// EncodedProjectDir is the pi / dsh layout: --Users-name-work-foo--
func EncodedProjectDir(cwd string) string {
	trimmed := strings.TrimLeft(cwd, `/\`)
	repl := strings.NewReplacer("/", "-", `\`, "-", ":", "-")
	return "--" + repl.Replace(trimmed) + "--"
}

// ClaudeProjectSlug is Claude Code layout: -Users-name-work-foo
func ClaudeProjectSlug(cwd string) string {
	return strings.ReplaceAll(filepath.ToSlash(cwd), "/", "-")
}

func PiDir(cwd string) string {
	return filepath.Join(PiSessionsRoot(), EncodedProjectDir(cwd))
}

func ClaudeDir(cwd string) string {
	return filepath.Join(ClaudeProjectsRoot(), ClaudeProjectSlug(cwd))
}

func DSHDir(cwd string) string {
	return filepath.Join(DSHSessionsRoot(), EncodedProjectDir(cwd))
}

// DecodeProjectDir reverses EncodedProjectDir / ClaudeProjectSlug best-effort.
func DecodeProjectDir(name string) string {
	n := strings.TrimSpace(name)
	n = strings.TrimPrefix(n, "--")
	n = strings.TrimSuffix(n, "--")
	n = strings.TrimPrefix(n, "-")
	if n == "" {
		return ""
	}
	return "/" + strings.ReplaceAll(n, "-", "/")
}
