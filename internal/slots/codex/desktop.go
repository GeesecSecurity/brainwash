package codex

import (
	"crypto/rand"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"brainwash/internal/ir"
	"brainwash/internal/jsonl"
	"brainwash/internal/paths"
	"brainwash/internal/textutil"

	_ "modernc.org/sqlite"
)

const desktopCLIVersion = "0.148.0-alpha.15"

func firstUserPreview(sess *ir.Session) string {
	for _, ev := range sess.Events {
		if ev.Role != ir.RoleUser || ev.Kind == ir.KindInject {
			continue
		}
		t := strings.TrimSpace(ev.Text)
		if t == "" {
			continue
		}
		return textutil.TruncateRunes(t, 240)
	}
	return strings.TrimSpace(sess.Title)
}

func registerDesktopThread(id, rolloutPath, cwd, title string, created, updated time.Time, preview string) error {
	if err := upsertSQLiteThread(id, rolloutPath, cwd, title, created, updated, preview); err != nil {
		return err
	}
	_ = upsertGlobalState(id, cwd, title)
	return nil
}

func upsertSQLiteThread(id, rolloutPath, cwd, title string, created, updated time.Time, preview string) error {
	dbPath := filepath.Join(paths.Home(), ".codex", "state_5.sqlite")
	if _, err := os.Stat(dbPath); err != nil {
		return nil
	}
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)")
	if err != nil {
		return err
	}
	defer db.Close()
	if created.IsZero() {
		created = time.Now()
	}
	if updated.IsZero() {
		updated = created
	}
	if title == "" {
		title = id
	}
	if preview == "" {
		preview = title
	}
	sec := created.Unix()
	msec := created.UnixMilli()
	usec := updated.Unix()
	umsec := updated.UnixMilli()
	_, err = db.Exec(`
INSERT INTO threads (
  id, rollout_path, created_at, updated_at, source, model_provider, cwd, title,
  sandbox_policy, approval_mode, tokens_used, has_user_event, archived,
  cli_version, first_user_message, memory_mode, model, reasoning_effort,
  created_at_ms, updated_at_ms, thread_source, preview, recency_at, recency_at_ms,
  history_mode, is_pinned
) VALUES (
  ?, ?, ?, ?, 'vscode', 'man', ?, ?,
  '{"type":"disabled"}', 'never', 0, 1, 0,
  ?, ?, 'enabled', 'gpt-5.4', 'medium',
  ?, ?, 'user', ?, ?, ?,
  'legacy', 0
)
ON CONFLICT(id) DO UPDATE SET
  rollout_path=excluded.rollout_path,
  updated_at=excluded.updated_at,
  updated_at_ms=excluded.updated_at_ms,
  title=excluded.title,
  cwd=excluded.cwd,
  first_user_message=excluded.first_user_message,
  preview=excluded.preview,
  recency_at=excluded.recency_at,
  recency_at_ms=excluded.recency_at_ms
`, id, rolloutPath, sec, usec, cwd, title, desktopCLIVersion, preview, msec, umsec, preview, usec, umsec)
	return err
}

func upsertGlobalState(threadID, cwd, title string) error {
	path := filepath.Join(paths.Home(), ".codex", ".codex-global-state.json")
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var root map[string]any
	if json.Unmarshal(raw, &root) != nil {
		return nil
	}
	if root == nil {
		root = map[string]any{}
	}
	projectID := ensureLocalProject(root, cwd)
	assign, _ := root["thread-project-assignments"].(map[string]any)
	if assign == nil {
		assign = map[string]any{}
	}
	if projectID != "" {
		assign[threadID] = map[string]any{"projectKind": "local", "projectId": projectID}
		root["thread-project-assignments"] = assign
	}
	roots := asStringSlice(root["electron-saved-workspace-roots"])
	if cwd != "" && !contains(roots, cwd) {
		root["electron-saved-workspace-roots"] = append(roots, cwd)
	}
	bak := path + ".bak"
	_ = os.WriteFile(bak, raw, 0o644)
	tmp := path + ".tmp-brainwash"
	out, err := json.MarshalIndent(root, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(tmp, out, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func ensureLocalProject(root map[string]any, cwd string) string {
	cwd = filepath.Clean(cwd)
	if cwd == "" || cwd == "." {
		return ""
	}
	projects, _ := root["local-projects"].(map[string]any)
	if projects == nil {
		projects = map[string]any{}
		root["local-projects"] = projects
	}
	for id, v := range projects {
		m, _ := v.(map[string]any)
		if m == nil {
			continue
		}
		for _, r := range asStringSlice(m["rootPaths"]) {
			if filepath.Clean(r) == cwd {
				return id
			}
		}
	}
	id := "local-" + randHex(16)
	now := time.Now().UnixMilli()
	name := filepath.Base(cwd)
	projects[id] = map[string]any{
		"id": id, "name": name, "rootPaths": []string{cwd},
		"createdAt": now, "updatedAt": now,
	}
	order := asStringSlice(root["project-order"])
	root["project-order"] = append([]string{id}, order...)
	return id
}

func asStringSlice(v any) []string {
	switch t := v.(type) {
	case []string:
		return t
	case []any:
		var out []string
		for _, x := range t {
			if s, ok := x.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

func contains(ss []string, v string) bool {
	for _, s := range ss {
		if s == v {
			return true
		}
	}
	return false
}

func ReindexMissing() (int, error) {
	root := paths.CodexSessionsRoot()
	n := 0
	return n, filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if !strings.HasPrefix(d.Name(), "rollout-") || !strings.HasSuffix(d.Name(), ".jsonl") {
			return nil
		}
		_ = RepairDesktopHistory(p)
		id, cwd, title, created := peekMeta(p)
		if id == "" {
			return nil
		}
		missing, err := threadMissing(id)
		if err != nil || !missing {
			return err
		}
		if title == "" {
			title = id
		}
		updated := jsonl.FileMod(p)
		if created.IsZero() {
			created = updated
		}
		if err := registerDesktopThread(id, p, cwd, title, created, updated, title); err != nil {
			return err
		}
		n++
		return nil
	})
}

func peekMeta(path string) (id, cwd, title string, created time.Time) {
	obj, err := jsonl.FirstObject(path)
	if err != nil || obj == nil {
		return
	}
	payload := obj
	if inner := jsonl.GetMap(obj, "payload"); inner != nil {
		payload = inner
	}
	id = jsonl.String(payload["id"])
	if id == "" {
		id = jsonl.String(payload["session_id"])
	}
	cwd = jsonl.String(payload["cwd"])
	title = jsonl.String(payload["name"])
	created = jsonl.TimeOf(payload["timestamp"])
	return
}

func threadMissing(id string) (bool, error) {
	dbPath := filepath.Join(paths.Home(), ".codex", "state_5.sqlite")
	if _, err := os.Stat(dbPath); err != nil {
		return true, nil
	}
	db, err := sql.Open("sqlite", dbPath+"?_pragma=busy_timeout(5000)")
	if err != nil {
		return true, err
	}
	defer db.Close()
	var n int
	err = db.QueryRow(`select count(*) from threads where id = ?`, id).Scan(&n)
	if err != nil {
		return true, err
	}
	return n == 0, nil
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	const hex = "0123456789abcdef"
	out := make([]byte, n*2)
	for i, x := range b {
		out[i*2] = hex[x>>4]
		out[i*2+1] = hex[x&0x0f]
	}
	return string(out)
}
