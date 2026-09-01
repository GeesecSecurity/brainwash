package pm

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/google/uuid"

	"brainwash/internal/ir"
)

const (
	FormatName    = "brainwash.packed-memory"
	FormatVersion = 1
	ManifestName  = "manifest.json"
	SessionName   = "session.json"
)

type Manifest struct {
	Format       string    `json:"format"`
	Version      int       `json:"version"`
	ExportedAt   time.Time `json:"exportedAt"`
	SourceSlot   ir.Slot   `json:"sourceSlot"`
	SourceID     string    `json:"sourceId"`
	SourcePath   string    `json:"sourcePath,omitempty"`
	CWD          string    `json:"cwd"`
	Title        string    `json:"title"`
	Events       int       `json:"events"`
	IncludeTools bool      `json:"includeTools"`
	Notes        []string  `json:"notes,omitempty"`
}

type Pack struct {
	Manifest Manifest    `json:"manifest"`
	Session  *ir.Session `json:"session"`
}

func WriteFile(path string, sess *ir.Session, includeTools bool) error {
	if sess == nil {
		return fmt.Errorf("nil session")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	return Encode(f, sess, includeTools)
}

func Encode(w io.Writer, sess *ir.Session, includeTools bool) error {
	zw := zip.NewWriter(w)
	defer zw.Close()
	man := Manifest{
		Format:       FormatName,
		Version:      FormatVersion,
		ExportedAt:   time.Now().UTC(),
		SourceSlot:   sess.Slot,
		SourceID:     sess.ID,
		SourcePath:   sess.SourcePath,
		CWD:          sess.CWD,
		Title:        sess.Title,
		Events:       len(sess.Events),
		IncludeTools: includeTools,
		Notes:        sess.Notes,
	}
	if err := writeJSON(zw, ManifestName, man); err != nil {
		return err
	}
	return writeJSON(zw, SessionName, sess)
}

func ReadFile(path string) (*Pack, error) {
	r, err := zip.OpenReader(path)
	if err != nil {
		return nil, fmt.Errorf("open packed memory %s: %w", path, err)
	}
	defer r.Close()
	return Decode(&r.Reader)
}

func Decode(r *zip.Reader) (*Pack, error) {
	files := map[string]*zip.File{}
	for _, f := range r.File {
		name := filepath.ToSlash(f.Name)
		files[name] = f
	}
	manFile, ok := files[ManifestName]
	if !ok {
		return nil, fmt.Errorf("packed memory missing %s", ManifestName)
	}
	sessFile, ok := files[SessionName]
	if !ok {
		return nil, fmt.Errorf("packed memory missing %s", SessionName)
	}
	var man Manifest
	if err := readJSON(manFile, &man); err != nil {
		return nil, err
	}
	if man.Format != "" && man.Format != FormatName {
		return nil, fmt.Errorf("unknown packed memory format %q", man.Format)
	}
	var sess ir.Session
	if err := readJSON(sessFile, &sess); err != nil {
		return nil, err
	}
	if sess.Slot == "" {
		sess.Slot = man.SourceSlot
	}
	if sess.Title == "" {
		sess.Title = man.Title
	}
	if sess.CWD == "" {
		sess.CWD = man.CWD
	}
	return &Pack{Manifest: man, Session: &sess}, nil
}

func writeJSON(zw *zip.Writer, name string, v any) error {
	w, err := zw.CreateHeader(&zip.FileHeader{
		Name:     name,
		Method:   zip.Deflate,
		Modified: time.Now(),
	})
	if err != nil {
		return err
	}
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	return enc.Encode(v)
}

func readJSON(f *zip.File, dest any) error {
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	return json.NewDecoder(rc).Decode(dest)
}

func DefaultFileName(_ *ir.Session) string {
	return uuid.NewString() + ".pm"
}
