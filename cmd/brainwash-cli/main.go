package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"strings"

	"brainwash/internal/ir"
	"brainwash/internal/service"
	"brainwash/internal/slots/codex"
	"brainwash/internal/version"
)

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	switch os.Args[1] {
	case "version", "-v", "--version":
		fmt.Println(version.String())
		return
	case "list":
		cmdList(os.Args[2:])
	case "show":
		cmdShow(os.Args[2:])
	case "clone":
		cmdClone(os.Args[2:])
	case "export":
		cmdExport(os.Args[2:])
	case "import":
		cmdImport(os.Args[2:])
	case "reindex-codex":
		cmdReindexCodex()
	case "serve":
		cmdServe(os.Args[2:])
	case "slots":
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{"slots": service.Slots()})
	case "help", "-h", "--help":
		usage()
	default:
		fmt.Fprintf(os.Stderr, "unknown command %q\n", os.Args[1])
		usage()
		os.Exit(2)
	}
}

func usage() {
	fmt.Fprintf(os.Stderr, `brainwash-cli — transfer coding-agent session memory between formats

Commands:
  list   [--slot pi|codex|claude|dsh] [--cwd DIR]
  show   --slot SLOT (--session ID|--path FILE|--latest) [--cwd DIR]
  clone  --from SLOT --to SLOT (--session ID|--path FILE|--latest|--all)
         [--cwd DIR] [--out-cwd DIR] [--no-tools] [--max-tool-chars N] [--dry-run]
  export --slot SLOT (--session ID|--path FILE|--latest) [--out FILE.pm] [--cwd DIR] [--no-tools]
  import --file FILE.pm [--file FILE.pm ...] [--to SLOT] [--out-cwd DIR] [--no-tools]
  reindex-codex
  serve  [--addr 127.0.0.1:7420]
  slots
  version

Omit --cwd to scan every project under that agent.
show / clone / export need an explicit session: --session, --path, --latest, or --all.
clone / import write into --out-cwd; if omitted they reuse the session's original project.
Clone narrates historical tools by default; --no-tools drops them.
Packed memory (.pm) is a zip of manifest.json + session.json.
Default export filename is a UUID in the current directory, not the session title.

`)
}

func cmdList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	cwd := fs.String("cwd", "", "project directory (empty = all projects)")
	slotName := fs.String("slot", "", "pi|codex|claude|dsh")
	asJSON := fs.Bool("json", true, "json output")
	_ = fs.Parse(args)
	items, err := service.List(service.ListRequest{CWD: *cwd, Slot: ir.Slot(*slotName)})
	die(err)
	if *asJSON {
		dump(items)
		return
	}
	for _, it := range items {
		fmt.Printf("%-8s  %s  %s  %s\n", it.Slot, it.ID, it.UpdatedAt.Format("2006-01-02 15:04"), it.Title)
	}
}

func cmdShow(args []string) {
	fs := flag.NewFlagSet("show", flag.ExitOnError)
	cwd := fs.String("cwd", "", "project directory")
	slotName := fs.String("slot", "", "pi|codex|claude|dsh")
	session := fs.String("session", "", "session id")
	path := fs.String("path", "", "session file path")
	latest := fs.Bool("latest", false, "latest session")
	_ = fs.Parse(args)
	if *slotName == "" {
		die(fmt.Errorf("--slot is required"))
	}
	if *session == "" && *path == "" && !*latest {
		die(fmt.Errorf("specify --session, --path, or --latest"))
	}
	sess, err := service.Show(service.ShowRequest{CWD: *cwd, Slot: ir.Slot(*slotName), Session: *session, Path: *path, Latest: *latest})
	die(err)
	dump(sess)
}

func cmdClone(args []string) {
	fs := flag.NewFlagSet("clone", flag.ExitOnError)
	cwd := fs.String("cwd", "", "source project directory (empty = all projects)")
	outCWD := fs.String("out-cwd", "", "destination project directory (default: session cwd)")
	from := fs.String("from", "", "source slot")
	to := fs.String("to", "", "destination slot")
	session := fs.String("session", "", "session id")
	path := fs.String("path", "", "session file path")
	latest := fs.Bool("latest", false, "latest session")
	all := fs.Bool("all", false, "all sessions")
	includeTools := fs.Bool("include-tools", true, "narrate historical tools (default on)")
	noTools := fs.Bool("no-tools", false, "drop historical tool traces")
	maxChars := fs.Int("max-tool-chars", 4000, "tool preview char limit")
	dry := fs.Bool("dry-run", false, "do not write")
	_ = fs.Parse(args)
	if *from == "" || *to == "" {
		die(fmt.Errorf("--from and --to are required"))
	}
	if *session == "" && *path == "" && !*latest && !*all {
		die(fmt.Errorf("specify --session, --path, --latest, or --all"))
	}
	narrate := *includeTools && !*noTools
	res, err := service.Clone(service.CloneRequest{
		CWD: *cwd, OutCWD: *outCWD, From: ir.Slot(*from), To: ir.Slot(*to),
		Session: *session, Path: *path, Latest: *latest, All: *all,
		IncludeTools: narrate, MaxToolChars: *maxChars, DryRun: *dry,
	})
	die(err)
	dump(res)
}

func cmdExport(args []string) {
	fs := flag.NewFlagSet("export", flag.ExitOnError)
	cwd := fs.String("cwd", "", "project directory")
	slotName := fs.String("slot", "", "pi|codex|claude|dsh")
	session := fs.String("session", "", "session id")
	path := fs.String("path", "", "session file path")
	out := fs.String("out", "", "output .pm path (default: ./<uuid>.pm)")
	latest := fs.Bool("latest", false, "latest session")
	includeTools := fs.Bool("include-tools", true, "keep historical tools in packed IR")
	noTools := fs.Bool("no-tools", false, "drop historical tool traces from packed IR")
	_ = fs.Parse(args)
	if *slotName == "" {
		die(fmt.Errorf("--slot is required"))
	}
	if *session == "" && *path == "" && !*latest {
		die(fmt.Errorf("specify --session, --path, or --latest"))
	}
	res, err := service.Export(service.ExportRequest{
		CWD: *cwd, Slot: ir.Slot(*slotName), Session: *session, Path: *path,
		Latest: *latest, Out: *out, IncludeTools: *includeTools && !*noTools,
	})
	die(err)
	dump(res)
}

func cmdImport(args []string) {
	fs := flag.NewFlagSet("import", flag.ExitOnError)
	to := fs.String("to", "", "destination slot (default: packed source slot)")
	outCWD := fs.String("out-cwd", "", "destination project directory")
	var files multiFlag
	fs.Var(&files, "file", "packed memory .pm (repeatable)")
	includeTools := fs.Bool("include-tools", true, "narrate historical tools")
	noTools := fs.Bool("no-tools", false, "drop historical tool traces")
	_ = fs.Parse(args)
	files = append(files, fs.Args()...)
	if len(files) == 0 {
		die(fmt.Errorf("at least one --file is required"))
	}
	res, err := service.Import(service.ImportRequest{
		Files: files, To: ir.Slot(*to), OutCWD: *outCWD, IncludeTools: *includeTools && !*noTools,
	})
	die(err)
	dump(res)
}

type multiFlag []string

func (m *multiFlag) String() string { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error {
	*m = append(*m, v)
	return nil
}

func cmdReindexCodex() {
	n, err := codex.ReindexMissing()
	die(err)
	dump(map[string]any{"registered": n})
}

func cmdServe(args []string) {
	fs := flag.NewFlagSet("serve", flag.ExitOnError)
	addr := fs.String("addr", "127.0.0.1:7420", "listen address")
	_ = fs.Parse(args)
	fmt.Fprintf(os.Stderr, "brainwash-cli serve %s\n", *addr)
	die(http.ListenAndServe(*addr, service.Handler()))
}

func dump(v any) {
	enc := json.NewEncoder(os.Stdout)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func die(err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	if !strings.HasSuffix(msg, "\n") {
		msg += "\n"
	}
	fmt.Fprint(os.Stderr, msg)
	os.Exit(1)
}
