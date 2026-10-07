package object

import (
	"strings"

	"skein/internal/canonical"
	"skein/internal/skeinerr"
)

func normalizePath(path string) (string, error) {
	p := canonical.NFC(path)
	if strings.ContainsRune(p, '\\') || strings.ContainsRune(p, 0) {
		return "", skeinerr.New(skeinerr.Malformed, "path must not contain NUL or backslash")
	}
	if strings.HasPrefix(p, "/") || strings.HasSuffix(p, "/") {
		return "", skeinerr.New(skeinerr.Malformed, "path must be relative with no leading or trailing slash")
	}
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." || part == ".." {
			return "", skeinerr.New(skeinerr.Malformed, "forbidden path component: "+part)
		}
	}
	return p, nil
}

func ASCIICasefold(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for _, r := range s {
		if r >= 'A' && r <= 'Z' {
			b.WriteRune(r + 32)
		} else {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func PathsConflict(a, b, pathMode string) bool {
	if a == b {
		return true
	}
	if pathMode == "case-insensitive" && ASCIICasefold(a) == ASCIICasefold(b) {
		return true
	}
	return false
}
