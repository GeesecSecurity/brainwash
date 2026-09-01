package textutil

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

var ws = regexp.MustCompile(`\s+`)

func Compact(s string) string {
	return strings.TrimSpace(ws.ReplaceAllString(s, " "))
}

func TruncateRunes(s string, n int) string {
	if n <= 0 || utf8.RuneCountInString(s) <= n {
		return s
	}
	r := []rune(s)
	return string(r[:n]) + "…"
}

func FirstLine(s string, n int) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		s = s[:i]
	}
	return TruncateRunes(Compact(s), n)
}

func NonEmpty(s string) bool {
	return strings.TrimSpace(s) != ""
}
