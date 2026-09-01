package jsonl

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/klauspost/compress/zstd"
)

func ReadLinesLimit(path string, max int, fn func([]byte) error) error {
	n := 0
	return ReadLines(path, func(b []byte) error {
		n++
		if max > 0 && n > max {
			return errStop
		}
		return fn(b)
	})
}

var errStop = fmt.Errorf("jsonl: stop")

func ReadLines(path string, fn func([]byte) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*64), 16*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		cp := make([]byte, len(line))
		copy(cp, line)
		if err := fn(cp); err != nil {
			return ignoreStop(err)
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return nil
}

func ignoreStop(err error) error {
	if err == nil || err == errStop {
		return nil
	}
	return err
}

func FirstObject(path string) (map[string]any, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 4096), 1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(line, &obj); err != nil {
			return nil, err
		}
		return obj, nil
	}
	return nil, sc.Err()
}

func WriteLines(path string, objects []any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	for _, obj := range objects {
		switch v := obj.(type) {
		case []byte:
			buf.Write(bytes.TrimRight(v, "\n"))
			buf.WriteByte('\n')
		default:
			if err := enc.Encode(obj); err != nil {
				return err
			}
		}
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

func AppendLine(path string, obj any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	enc := json.NewEncoder(f)
	enc.SetEscapeHTML(false)
	return enc.Encode(obj)
}

func ReadZstdLines(path string, fn func([]byte) error) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	dec, err := zstd.NewReader(f)
	if err != nil {
		return err
	}
	defer dec.Close()
	sc := bufio.NewScanner(dec)
	sc.Buffer(make([]byte, 0, 1024*64), 32*1024*1024)
	for sc.Scan() {
		line := bytes.TrimSpace(sc.Bytes())
		if len(line) == 0 {
			continue
		}
		cp := make([]byte, len(line))
		copy(cp, line)
		if err := fn(cp); err != nil {
			return ignoreStop(err)
		}
	}
	return sc.Err()
}

func WriteZstdLines(path string, objects []any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	var raw bytes.Buffer
	enc := json.NewEncoder(&raw)
	enc.SetEscapeHTML(false)
	for _, obj := range objects {
		if err := enc.Encode(obj); err != nil {
			return err
		}
	}
	var out bytes.Buffer
	w, err := zstd.NewWriter(&out)
	if err != nil {
		return err
	}
	if _, err := w.Write(raw.Bytes()); err != nil {
		_ = w.Close()
		return err
	}
	if err := w.Close(); err != nil {
		return err
	}
	return os.WriteFile(path, out.Bytes(), 0o644)
}

func AsMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

func String(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case json.Number:
		return t.String()
	case float64:
		if t == float64(int64(t)) {
			return strconv.FormatInt(int64(t), 10)
		}
		return strconv.FormatFloat(t, 'f', -1, 64)
	case bool:
		if t {
			return "true"
		}
		return "false"
	default:
		b, err := json.Marshal(t)
		if err != nil {
			return fmt.Sprint(t)
		}
		return string(b)
	}
}

func Get(m map[string]any, keys ...string) any {
	cur := any(m)
	for _, k := range keys {
		obj := AsMap(cur)
		if obj == nil {
			return nil
		}
		cur = obj[k]
	}
	return cur
}

func GetString(m map[string]any, keys ...string) string {
	return String(Get(m, keys...))
}

func GetBool(m map[string]any, keys ...string) bool {
	v := Get(m, keys...)
	b, _ := v.(bool)
	return b
}

func GetMap(m map[string]any, keys ...string) map[string]any {
	return AsMap(Get(m, keys...))
}

func GetSlice(m map[string]any, keys ...string) []any {
	v := Get(m, keys...)
	s, _ := v.([]any)
	return s
}

func TimeOf(v any) time.Time {
	switch t := v.(type) {
	case string:
		if t == "" {
			return time.Time{}
		}
		if tm, err := time.Parse(time.RFC3339Nano, t); err == nil {
			return tm
		}
		if tm, err := time.Parse(time.RFC3339, t); err == nil {
			return tm
		}
	case float64:
		if t > 1e12 {
			return time.UnixMilli(int64(t))
		}
		if t > 1e9 {
			return time.Unix(int64(t), 0)
		}
	case json.Number:
		n, _ := t.Float64()
		return TimeOf(n)
	}
	return time.Time{}
}

func CompactJSON(v any, limit int) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Sprint(v)
	}
	s := string(b)
	if limit > 0 && len(s) > limit {
		return s[:limit] + "\n…(truncated)…"
	}
	return s
}

func isImagePlaceholder(s string) bool {
	t := strings.ToLower(strings.TrimSpace(s))
	switch t {
	case "<image>", "</image>", "image", "[image]", "(image)", "![image]":
		return true
	}
	return false
}

func ImageURLs(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	var out []string
	for _, item := range arr {
		m := AsMap(item)
		if m == nil {
			continue
		}
		switch String(m["type"]) {
		case "input_image", "image", "image_url":
			u := String(m["image_url"])
			if u == "" {
				u = String(m["url"])
			}
			if u == "" {
				if nested := AsMap(m["image_url"]); nested != nil {
					u = String(nested["url"])
				}
			}
			if u == "" {
				u = String(m["source"])
			}
			if u != "" {
				out = append(out, u)
			}
		}
	}
	return out
}

func TextContent(v any) string {
	switch t := v.(type) {
	case nil:
		return ""
	case string:
		return t
	case []any:
		var parts []string
		for _, item := range t {
			m := AsMap(item)
			if m == nil {
				if s := String(item); s != "" {
					parts = append(parts, s)
				}
				continue
			}
			typ := String(m["type"])
			switch typ {
			case "text", "input_text", "output_text":
				s := String(m["text"])
				if s == "" || isImagePlaceholder(s) {
					continue
				}
				parts = append(parts, s)
			case "input_image", "image", "image_url":
				continue
			case "thinking", "reasoning":
				if s := String(m["thinking"]); s != "" {
					parts = append(parts, s)
				} else if s := String(m["text"]); s != "" {
					parts = append(parts, s)
				}
			default:
				if s := String(m["text"]); s != "" {
					parts = append(parts, s)
				}
			}
		}
		return strings.Join(parts, "\n")
	case map[string]any:
		if s := String(t["text"]); s != "" {
			return s
		}
		return CompactJSON(t, 400)
	default:
		return String(t)
	}
}

func FileMod(path string) time.Time {
	st, err := os.Stat(path)
	if err != nil {
		return time.Now()
	}
	return st.ModTime()
}

func FileSize(path string) int64 {
	st, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return st.Size()
}

func MustUUID() string {
	b := make([]byte, 16)
	f, err := os.Open("/dev/urandom")
	if err != nil {
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	defer f.Close()
	_, _ = f.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func Hex8() string {
	u := MustUUID()
	return strings.ReplaceAll(u, "-", "")[:8]
}
