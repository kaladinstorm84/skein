package canonical

import (
	"bytes"
	"encoding/json"
	"io"
	"os"
	"regexp"
	"strconv"
	"strings"

	"skein/internal/skeinerr"
)

var rfc3339msZ = regexp.MustCompile(`^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\.[0-9]{3}Z$`)

func ParseRFC3339(value string) (string, error) {
	if !rfc3339msZ.MatchString(value) {
		return "", skeinerr.New(skeinerr.Malformed, "timestamp must be RFC 3339 UTC with millisecond precision and Z")
	}
	return value, nil
}

func DecodeJSON(data []byte) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, skeinerr.New(skeinerr.Malformed, err.Error())
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		return nil, skeinerr.New(skeinerr.Malformed, "trailing JSON")
	}
	return ConvertJSON(v)
}

func DecodeJSONFile(path string) (any, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, skeinerr.New(skeinerr.IO, err.Error())
	}
	return DecodeJSON(b)
}

func ConvertJSON(v any) (any, error) {
	switch t := v.(type) {
	case nil, bool, string:
		return t, nil
	case json.Number:
		s := t.String()
		if strings.ContainsAny(s, ".eE") {
			f, err := strconv.ParseFloat(s, 64)
			if err != nil {
				return nil, skeinerr.New(skeinerr.Malformed, "invalid number")
			}
			return f, nil
		}
		n, err := strconv.ParseInt(s, 10, 64)
		if err != nil {
			return nil, skeinerr.New(skeinerr.Malformed, "invalid integer")
		}
		return n, nil
	case []any:
		out := make([]any, len(t))
		for i, item := range t {
			c, err := ConvertJSON(item)
			if err != nil {
				return nil, err
			}
			out[i] = c
		}
		return out, nil
	case map[string]any:
		out := make(map[string]any, len(t))
		for k, item := range t {
			c, err := ConvertJSON(item)
			if err != nil {
				return nil, err
			}
			out[k] = c
		}
		return out, nil
	default:
		return nil, skeinerr.New(skeinerr.Malformed, "unsupported JSON value")
	}
}

func AsMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func AsSlice(v any) ([]any, bool) {
	s, ok := v.([]any)
	return s, ok
}

func AsString(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok
}

func AsInt(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	default:
		return 0, false
	}
}

func AsBool(v any) (bool, bool) {
	b, ok := v.(bool)
	return b, ok
}
