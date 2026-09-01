package service

import (
	"os"
	"path/filepath"
	"testing"

	"brainwash/internal/ir"
	"brainwash/internal/pm"
	"brainwash/internal/slot"
	_ "brainwash/internal/slots/pi"
)

func TestExportImportPackedMemory(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd := filepath.Join(home, "proj")
	_ = os.MkdirAll(cwd, 0o755)
	src := &ir.Session{
		ID: "s1", Slot: ir.SlotPi, CWD: cwd, Title: "login timeout",
		Events: []ir.Event{
			{Role: ir.RoleUser, Kind: ir.KindInput, Text: "fix login timeout"},
			{Role: ir.RoleAssistant, Text: "patched the handler"},
		},
	}
	p := slot.Must(ir.SlotPi)
	path, err := p.Write(src, cwd, ir.WriteOptions{IncludeTools: true})
	if err != nil {
		t.Fatal(err)
	}
	pmPath := filepath.Join(home, "out.pm")
	res, err := Export(ExportRequest{Slot: ir.SlotPi, Path: path, Out: pmPath, IncludeTools: true})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(res.Path); err != nil {
		t.Fatal(err)
	}
	if pack, err := pm.ReadFile(pmPath); err != nil || pack.Session.Title == "" {
		t.Fatalf("read pack: %v", err)
	}
	outCWD := filepath.Join(home, "imported")
	_ = os.MkdirAll(outCWD, 0o755)
	got, err := Import(ImportRequest{Files: []string{pmPath}, To: ir.SlotPi, OutCWD: outCWD, IncludeTools: true})
	if err != nil || len(got) != 1 {
		t.Fatalf("import: %v %+v", err, got)
	}
	refs, err := p.List(outCWD)
	if err != nil || len(refs) == 0 {
		t.Fatalf("list imported: %v %v", err, refs)
	}
}
