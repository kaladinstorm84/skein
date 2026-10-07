package canonical

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	"golang.org/x/text/unicode/norm"

	"skein/internal/skeinerr"
)

const (
	IntMin int64 = -9007199254740991
	IntMax int64 = 9007199254740991
)

func NFC(s string) string {
	return norm.NFC.String(s)
}

func Canonicalize(value any) ([]byte, error) {
	var b strings.Builder
	if err := writeCanonical(&b, value); err != nil {
		return nil, err
	}
	return []byte(b.String()), nil
}

func writeCanonical(b *strings.Builder, value any) error {
	switch v := value.(type) {
	case nil:
		b.WriteString("null")
		return nil
	case bool:
		if v {
			b.WriteString("true")
		} else {
			b.WriteString("false")
		}
		return nil
	case int:
		return writeInt(b, int64(v))
	case int8:
		return writeInt(b, int64(v))
	case int16:
		return writeInt(b, int64(v))
	case int32:
		return writeInt(b, int64(v))
	case int64:
		return writeInt(b, v)
	case uint:
		return writeInt(b, int64(v))
	case uint64:
		if v > uint64(IntMax) {
			return skeinerr.New(skeinerr.Malformed, fmt.Sprintf("integer out of IEEE-safe range: %d", v))
		}
		return writeInt(b, int64(v))
	case float32, float64:
		return skeinerr.New(skeinerr.Malformed, "floating-point values are forbidden")
	case string:
		b.Write(EncodeJSONString(v))
		return nil
	case []any:
		b.WriteByte('[')
		for i, item := range v {
			if i > 0 {
				b.WriteByte(',')
			}
			if err := writeCanonical(b, item); err != nil {
				return err
			}
		}
		b.WriteByte(']')
		return nil
	case map[string]any:
		type kv struct {
			key string
			val any
		}
		items := make([]kv, 0, len(v))
		seen := make(map[string]struct{}, len(v))
		for k, val := range v {
			nk := NFC(k)
			if _, ok := seen[nk]; ok {
				return skeinerr.New(skeinerr.Malformed, "duplicate object key after NFC: "+nk)
			}
			seen[nk] = struct{}{}
			items = append(items, kv{key: nk, val: val})
		}
		sort.Slice(items, func(i, j int) bool {
			return codePointLess(items[i].key, items[j].key)
		})
		b.WriteByte('{')
		for i, it := range items {
			if i > 0 {
				b.WriteByte(',')
			}
			b.Write(EncodeJSONString(it.key))
			b.WriteByte(':')
			if err := writeCanonical(b, it.val); err != nil {
				return err
			}
		}
		b.WriteByte('}')
		return nil
	default:
		return skeinerr.New(skeinerr.Malformed, fmt.Sprintf("unsupported canonical JSON type: %T", value))
	}
}

func writeInt(b *strings.Builder, n int64) error {
	if n < IntMin || n > IntMax {
		return skeinerr.New(skeinerr.Malformed, fmt.Sprintf("integer out of IEEE-safe range: %d", n))
	}
	b.WriteString(strconv.FormatInt(n, 10))
	return nil
}

func codePointLess(a, b string) bool {
	for len(a) > 0 && len(b) > 0 {
		ra, wa := utf8.DecodeRuneInString(a)
		rb, wb := utf8.DecodeRuneInString(b)
		if ra != rb {
			return ra < rb
		}
		a = a[wa:]
		b = b[wb:]
	}
	return len(a) < len(b)
}

func EncodeJSONString(s string) []byte {
	s = NFC(s)
	var b strings.Builder
	b.Grow(len(s) + 2)
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"':
			b.WriteString(`\"`)
		case '\\':
			b.WriteString(`\\`)
		case '\b':
			b.WriteString(`\b`)
		case '\t':
			b.WriteString(`\t`)
		case '\n':
			b.WriteString(`\n`)
		case '\f':
			b.WriteString(`\f`)
		case '\r':
			b.WriteString(`\r`)
		default:
			if r < 0x20 {
				b.WriteString(fmt.Sprintf(`\u%04x`, r))
			} else {
				b.WriteRune(r)
			}
		}
	}
	b.WriteByte('"')
	return []byte(b.String())
}
