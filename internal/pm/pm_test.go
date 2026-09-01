package pm

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"brainwash/internal/ir"
)

func TestDefaultFileNameIsUUID(t *testing.T) {
	name := DefaultFileName(&ir.Session{ID: "src", Title: "fix login", Slot: ir.SlotPi})
	if !strings.HasSuffix(name, ".pm") {
		t.Fatalf("suffix: %s", name)
	}
	id := strings.TrimSuffix(name, ".pm")
	if _, err := uuid.Parse(id); err != nil {
		t.Fatalf("want uuid filename, got %s: %v", name, err)
	}
	if strings.Contains(strings.ToLower(name), "login") {
		t.Fatalf("must not use session title: %s", name)
	}
}

func TestRoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "demo.pm")
	sess := &ir.Session{
		ID: "abc", Slot: ir.SlotPi, CWD: "/tmp/proj", Title: "fix login",
		SourcePath: "/tmp/src.jsonl", CreatedAt: time.Now(), UpdatedAt: time.Now(),
		Events: []ir.Event{
			{Role: ir.RoleUser, Kind: ir.KindInput, Text: "hello", Timestamp: time.Now()},
		},
	}
	if err := WriteFile(path, sess, true); err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(path)
	if err != nil || st.Size() == 0 {
		t.Fatalf("stat: %v size=%d", err, st.Size())
	}
	pack, err := ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if pack.Manifest.Format != FormatName || pack.Session.Title != "fix login" {
		t.Fatalf("%+v", pack.Manifest)
	}
	if pack.Session.Events[0].Text != "hello" {
		t.Fatalf("events=%+v", pack.Session.Events)
	}
}
