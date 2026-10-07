package object

import (
	"sort"
	"unicode/utf8"
)

func sortTreeEntries(entries []any) {
	sort.Slice(entries, func(i, j int) bool {
		a := entries[i].(map[string]any)["path"].(string)
		b := entries[j].(map[string]any)["path"].(string)
		return codePointLess(a, b)
	})
}

func codePointLess(a, b string) bool {
	for len(a) > 0 && len(b) > 0 {
		ra, wa := utf8.DecodeRuneInString(a)
		rb, wb := utf8.DecodeRuneInString(b)
		if ra != rb {
			return ra < rb
		}
		a, b = a[wa:], b[wb:]
	}
	return len(a) < len(b)
}
