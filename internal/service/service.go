package service

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"brainwash/internal/injects"
	"brainwash/internal/ir"
	"brainwash/internal/paths"
	"brainwash/internal/pm"
	"brainwash/internal/slot"

	_ "brainwash/internal/slots/claude"
	_ "brainwash/internal/slots/codex"
	_ "brainwash/internal/slots/dsh"
	_ "brainwash/internal/slots/pi"
)

type ListRequest struct {
	CWD  string   `json:"cwd"`
	Slot ir.Slot  `json:"slot,omitempty"`
	Slots []ir.Slot `json:"slots,omitempty"`
}

type ShowRequest struct {
	CWD     string  `json:"cwd"`
	Slot    ir.Slot `json:"slot"`
	Session string  `json:"session"`
	Path    string  `json:"path"`
	Latest  bool    `json:"latest"`
}

type CloneRequest struct {
	CWD          string  `json:"cwd"`
	OutCWD       string  `json:"outCwd"`
	From         ir.Slot `json:"from"`
	To           ir.Slot `json:"to"`
	Session      string  `json:"session"`
	Path         string  `json:"path"`
	Latest       bool    `json:"latest"`
	All          bool    `json:"all"`
	IncludeTools bool   `json:"includeTools"`
	MaxToolChars int    `json:"maxToolChars"`
	NamePrefix   string `json:"namePrefix"`
	DryRun       bool   `json:"dryRun"`
}

type CloneResult struct {
	SourceID   string `json:"sourceId"`
	SourcePath string `json:"sourcePath"`
	DestPath   string `json:"destPath"`
	Events     int    `json:"events"`
	Title      string `json:"title"`
}

type ExportRequest struct {
	CWD          string  `json:"cwd"`
	Slot         ir.Slot `json:"slot"`
	Session      string  `json:"session"`
	Path         string  `json:"path"`
	Latest       bool    `json:"latest"`
	Out          string  `json:"out"`
	IncludeTools bool    `json:"includeTools"`
}

type ExportResult struct {
	Path   string `json:"path"`
	Title  string `json:"title"`
	Slot   ir.Slot `json:"slot"`
	Events int    `json:"events"`
}

type ImportRequest struct {
	Files        []string `json:"files"`
	To           ir.Slot  `json:"to"`
	OutCWD       string   `json:"outCwd"`
	IncludeTools bool     `json:"includeTools"`
	NamePrefix   string   `json:"namePrefix"`
}

type ImportResult struct {
	PackedPath string  `json:"packedPath"`
	SourceSlot ir.Slot `json:"sourceSlot"`
	DestSlot   ir.Slot `json:"destSlot"`
	DestPath   string  `json:"destPath"`
	Title      string  `json:"title"`
	Events     int     `json:"events"`
	CWD        string  `json:"cwd"`
}

func List(req ListRequest) ([]ir.SessionRef, error) {
	cwd := strings.TrimSpace(req.CWD)
	if cwd != "" {
		cwd = paths.ResolveCWD(cwd)
	}
	wanted := req.Slots
	if req.Slot != "" {
		wanted = append(wanted, req.Slot)
	}
	if len(wanted) == 0 {
		wanted = slot.Names()
	}
	var out []ir.SessionRef
	for _, name := range wanted {
		p, err := slot.Get(name)
		if err != nil {
			return nil, err
		}
		items, err := p.List(cwd)
		if err != nil {
			return nil, err
		}
		out = append(out, items...)
	}
	sort.Slice(out, func(i, j int) bool {
		return out[i].UpdatedAt.After(out[j].UpdatedAt)
	})
	return out, nil
}

func Show(req ShowRequest) (*ir.Session, error) {
	p, err := slot.Get(req.Slot)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(req.Path) != "" {
		return prepareSession(p.Load(ir.SessionRef{
			Slot: req.Slot, ID: req.Session, Path: req.Path, CWD: req.CWD,
		}))
	}
	cwd := strings.TrimSpace(req.CWD)
	if cwd != "" {
		cwd = paths.ResolveCWD(cwd)
	}
	refs, err := p.List(cwd)
	if err != nil {
		return nil, err
	}
	ref, err := pick(refs, req.Session, req.Latest, false)
	if err != nil {
		return nil, err
	}
	return prepareSession(p.Load(ref))
}

func prepareSession(sess *ir.Session, err error) (*ir.Session, error) {
	if err != nil || sess == nil {
		return sess, err
	}
	for i := range sess.Events {
		injects.PrepareEvent(&sess.Events[i])
	}
	return sess, nil
}

func Clone(req CloneRequest) ([]CloneResult, error) {
	cwd := strings.TrimSpace(req.CWD)
	if cwd != "" {
		cwd = paths.ResolveCWD(cwd)
	}
	outCWD := strings.TrimSpace(req.OutCWD)
	if outCWD != "" {
		outCWD = paths.ResolveCWD(outCWD)
	}
	src, err := slot.Get(req.From)
	if err != nil {
		return nil, err
	}
	dst, err := slot.Get(req.To)
	if err != nil {
		return nil, err
	}
	var selected []ir.SessionRef
	if strings.TrimSpace(req.Path) != "" {
		selected = []ir.SessionRef{{
			Slot: req.From, ID: req.Session, Path: req.Path, CWD: cwd,
		}}
	} else {
		refs, err := src.List(cwd)
		if err != nil {
			return nil, err
		}
		selected, err = pickAll(refs, req.Session, req.Latest, req.All)
		if err != nil {
			return nil, err
		}
	}
	opt := ir.WriteOptions{
		IncludeTools: req.IncludeTools,
		MaxToolChars: req.MaxToolChars,
		NamePrefix:   req.NamePrefix,
		DryRun:       req.DryRun,
	}
	var results []CloneResult
	for _, ref := range selected {
		sess, err := src.Load(ref)
		if err != nil {
			return nil, err
		}
		dest := outCWD
		if dest == "" {
			dest = sess.CWD
		}
		if dest == "" {
			dest = paths.ResolveCWD("")
		}
		path, err := dst.Write(sess, dest, opt)
		if err != nil {
			return nil, err
		}
		results = append(results, CloneResult{
			SourceID: sess.ID, SourcePath: sess.SourcePath, DestPath: path,
			Events: len(sess.Events), Title: sess.Title,
		})
	}
	return results, nil
}

func Export(req ExportRequest) (*ExportResult, error) {
	sess, err := Show(ShowRequest{
		CWD: req.CWD, Slot: req.Slot, Session: req.Session, Path: req.Path, Latest: req.Latest,
	})
	if err != nil {
		return nil, err
	}
	out := strings.TrimSpace(req.Out)
	if out == "" {
		out = pm.DefaultFileName(sess)
	}
	if !strings.HasSuffix(strings.ToLower(out), ".pm") {
		out += ".pm"
	}
	if err := pm.WriteFile(out, sess, req.IncludeTools); err != nil {
		return nil, err
	}
	abs, _ := filepathAbs(out)
	return &ExportResult{Path: abs, Title: sess.Title, Slot: sess.Slot, Events: len(sess.Events)}, nil
}

func Import(req ImportRequest) ([]ImportResult, error) {
	if len(req.Files) == 0 {
		return nil, fmt.Errorf("no packed memory files")
	}
	outCWD := strings.TrimSpace(req.OutCWD)
	if outCWD != "" {
		outCWD = paths.ResolveCWD(outCWD)
	}
	opt := ir.WriteOptions{
		IncludeTools: req.IncludeTools,
		NamePrefix:   req.NamePrefix,
	}
	if opt.NamePrefix == "" {
		opt.NamePrefix = "[packed memory] "
	}
	var results []ImportResult
	for _, file := range req.Files {
		pack, err := pm.ReadFile(file)
		if err != nil {
			return nil, err
		}
		sess := pack.Session
		destSlot := req.To
		if destSlot == "" {
			destSlot = pack.Manifest.SourceSlot
			if destSlot == "" {
				destSlot = sess.Slot
			}
		}
		dst, err := slot.Get(destSlot)
		if err != nil {
			return nil, err
		}
		dest := outCWD
		if dest == "" {
			dest = sess.CWD
		}
		if dest == "" {
			dest = paths.ResolveCWD("")
		}
		sess.Notes = append(sess.Notes, "Imported from packed memory "+file)
		path, err := dst.Write(sess, dest, opt)
		if err != nil {
			return nil, err
		}
		results = append(results, ImportResult{
			PackedPath: file, SourceSlot: pack.Manifest.SourceSlot, DestSlot: destSlot,
			DestPath: path, Title: sess.Title, Events: len(sess.Events), CWD: dest,
		})
	}
	return results, nil
}

func filepathAbs(p string) (string, error) {
	return paths.ResolveCWD(p), nil
}

func pick(refs []ir.SessionRef, id string, latest, all bool) (ir.SessionRef, error) {
	items, err := pickAll(refs, id, latest, all)
	if err != nil {
		return ir.SessionRef{}, err
	}
	return items[0], nil
}

func pickAll(refs []ir.SessionRef, id string, latest, all bool) ([]ir.SessionRef, error) {
	if len(refs) == 0 {
		return nil, fmt.Errorf("no sessions found")
	}
	sort.Slice(refs, func(i, j int) bool { return refs[i].UpdatedAt.After(refs[j].UpdatedAt) })
	if all {
		return refs, nil
	}
	if id != "" {
		for _, r := range refs {
			if r.ID == id || strings.HasPrefix(r.ID, id) || strings.Contains(r.Path, id) {
				return []ir.SessionRef{r}, nil
			}
		}
		return nil, fmt.Errorf("session %q not found", id)
	}
	if latest {
		return []ir.SessionRef{refs[0]}, nil
	}
	return nil, fmt.Errorf("specify --session, --path, --latest, or --all")
}

func Slots() []map[string]string {
	var out []map[string]string
	for _, p := range slot.All() {
		out = append(out, map[string]string{"name": string(p.Name()), "label": p.Label()})
	}
	return out
}

func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"ok": true, "slots": Slots(), "time": time.Now()})
	})
	mux.HandleFunc("/v1/slots", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, map[string]any{"slots": Slots()})
	})
	mux.HandleFunc("/v1/list", func(w http.ResponseWriter, r *http.Request) {
		var req ListRequest
		if err := decode(r, &req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		items, err := List(req)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"sessions": items})
	})
	mux.HandleFunc("/v1/show", func(w http.ResponseWriter, r *http.Request) {
		var req ShowRequest
		if err := decode(r, &req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		sess, err := Show(req)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, sess)
	})
	mux.HandleFunc("/v1/clone", func(w http.ResponseWriter, r *http.Request) {
		var req CloneRequest
		if err := decode(r, &req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		res, err := Clone(req)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"results": res})
	})
	mux.HandleFunc("/v1/export", func(w http.ResponseWriter, r *http.Request) {
		var req ExportRequest
		if err := decode(r, &req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		res, err := Export(req)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, res)
	})
	mux.HandleFunc("/v1/import", func(w http.ResponseWriter, r *http.Request) {
		var req ImportRequest
		if err := decode(r, &req); err != nil {
			http.Error(w, err.Error(), 400)
			return
		}
		res, err := Import(req)
		if err != nil {
			http.Error(w, err.Error(), 500)
			return
		}
		writeJSON(w, map[string]any{"results": res})
	})
	return mux
}

func decode(r *http.Request, dest any) error {
	if r.Method == http.MethodGet {
		q := r.URL.Query()
		req := map[string]any{
			"cwd": q.Get("cwd"), "slot": q.Get("slot"), "from": q.Get("from"),
			"to": q.Get("to"), "session": q.Get("session"),
			"path": q.Get("path"),
			"latest": q.Get("latest") == "1" || q.Get("latest") == "true",
			"all": q.Get("all") == "1" || q.Get("all") == "true",
			"includeTools": q.Get("includeTools") == "1" || q.Get("includeTools") == "true",
			"outCwd": q.Get("outCwd"),
		}
		b, _ := json.Marshal(req)
		return json.Unmarshal(b, dest)
	}
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(dest)
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}
