package jsonl

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"

	"github.com/klauspost/compress/zstd"
)

const (
	DefaultPeekLines = 80
	DefaultPeekBytes = 256 * 1024
)

func PeekObjects(path string, maxLines int, maxBytes int) []map[string]any {
	if maxLines <= 0 {
		maxLines = DefaultPeekLines
	}
	if maxBytes <= 0 {
		maxBytes = DefaultPeekBytes
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	return scanPeek(bufio.NewReaderSize(f, 32*1024), maxLines, maxBytes)
}

func PeekZstdObjects(path string, maxLines int, maxBytes int) []map[string]any {
	if maxLines <= 0 {
		maxLines = DefaultPeekLines
	}
	if maxBytes <= 0 {
		maxBytes = DefaultPeekBytes
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	dec, err := zstd.NewReader(f)
	if err != nil {
		return nil
	}
	defer dec.Close()
	return scanPeek(bufio.NewReaderSize(dec, 32*1024), maxLines, maxBytes)
}

func scanPeek(r *bufio.Reader, maxLines, maxBytes int) []map[string]any {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	var out []map[string]any
	n := 0
	bytesRead := 0
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		bytesRead += len(line)
		n++
		var m map[string]any
		if json.Unmarshal(line, &m) == nil {
			out = append(out, m)
		}
		if n >= maxLines || bytesRead >= maxBytes {
			break
		}
	}
	return out
}

// ScanFor scans until pred matches, up to maxLines/maxBytes.
func ScanFor(path string, maxLines, maxBytes int, pred func(map[string]any) bool) map[string]any {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 64*1024), 4*1024*1024)
	n, bytesRead := 0, 0
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		bytesRead += len(line)
		n++
		var m map[string]any
		if json.Unmarshal(line, &m) == nil && pred(m) {
			return m
		}
		if maxLines > 0 && n >= maxLines {
			break
		}
		if maxBytes > 0 && bytesRead >= maxBytes {
			break
		}
	}
	return nil
}

func TitleFromUserText(text string) string {
	t := compactWS(text)
	if t == "" {
		return ""
	}
	if len(t) > 80 {
		return t[:80]
	}
	return t
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
