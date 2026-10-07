package state

import (
	"sort"
	"strings"

	"skein/internal/canonical"
	"skein/internal/object"
	"skein/internal/skeinerr"
)

type Hunk struct {
	StartLine int
	EndLine   int
	Lines     []string
}

func LogicalLines(text, lineEnding string) ([]string, error) {
	sep, err := separator(lineEnding)
	if err != nil {
		return nil, err
	}
	if text == "" {
		return []string{}, nil
	}
	return strings.Split(text, sep), nil
}

func JoinLines(lines []string, lineEnding string) (string, error) {
	sep, err := separator(lineEnding)
	if err != nil {
		return "", err
	}
	return strings.Join(lines, sep), nil
}

func separator(lineEnding string) (string, error) {
	switch lineEnding {
	case "lf":
		return "\n", nil
	case "crlf":
		return "\r\n", nil
	default:
		return "", skeinerr.New(skeinerr.Malformed, "line_ending must be lf or crlf")
	}
}

func HunksConflict(a, b Hunk) (bool, error) {
	s1, e1, s2, e2 := a.StartLine, a.EndLine, b.StartLine, b.EndLine
	if s1 > e1 || s2 > e2 || s1 < 0 || s2 < 0 {
		return false, skeinerr.New(skeinerr.Malformed, "hunk range must satisfy 0 <= start <= end")
	}
	insert1 := s1 == e1
	insert2 := s2 == e2
	if !insert1 && !insert2 {
		return max(s1, s2) < min(e1, e2), nil
	}
	if insert1 && insert2 {
		return s1 == s2, nil
	}
	p := s1
	s, e := s2, e2
	if !insert1 {
		p = s2
		s, e = s1, e1
	}
	return s <= p && p <= e, nil
}

func DetectHunkConflicts(hunks []Hunk) ([][2]int, error) {
	var conflicts [][2]int
	for i := 0; i < len(hunks); i++ {
		for j := i + 1; j < len(hunks); j++ {
			ok, err := HunksConflict(hunks[i], hunks[j])
			if err != nil {
				return nil, err
			}
			if ok {
				conflicts = append(conflicts, [2]int{i, j})
			}
		}
	}
	return conflicts, nil
}

func ApplyHunks(text, lineEnding string, hunks []Hunk) (string, error) {
	conf, err := DetectHunkConflicts(hunks)
	if err != nil {
		return "", err
	}
	if len(conf) > 0 {
		return "", skeinerr.New(skeinerr.Conflict, "hunk ranges conflict")
	}
	lines, err := LogicalLines(text, lineEnding)
	if err != nil {
		return "", err
	}
	type indexed struct {
		i int
		h Hunk
	}
	ordered := make([]indexed, len(hunks))
	for i, h := range hunks {
		ordered[i] = indexed{i, h}
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].h.StartLine != ordered[j].h.StartLine {
			return ordered[i].h.StartLine > ordered[j].h.StartLine
		}
		return ordered[i].i > ordered[j].i
	})
	for _, it := range ordered {
		h := it.h
		if h.EndLine > len(lines) || h.StartLine > len(lines) {
			return "", skeinerr.New(skeinerr.Malformed, "hunk range exceeds file line count")
		}
		newLines := make([]string, len(h.Lines))
		for i, line := range h.Lines {
			newLines[i] = canonical.NFC(line)
		}
		out := append([]string{}, lines[:h.StartLine]...)
		out = append(out, newLines...)
		out = append(out, lines[h.EndLine:]...)
		lines = out
	}
	return JoinLines(lines, lineEnding)
}

func HunksFromAny(v any) ([]Hunk, error) {
	arr, ok := canonical.AsSlice(v)
	if !ok {
		return nil, skeinerr.New(skeinerr.Malformed, "hunks must be an array")
	}
	out := make([]Hunk, 0, len(arr))
	for _, item := range arr {
		m, ok := canonical.AsMap(item)
		if !ok {
			return nil, skeinerr.New(skeinerr.Malformed, "hunk must be an object")
		}
		start, ok1 := canonical.AsInt(m["start_line"])
		end, ok2 := canonical.AsInt(m["end_line"])
		if !ok1 || !ok2 {
			return nil, skeinerr.New(skeinerr.Malformed, "hunk range required")
		}
		h := Hunk{StartLine: int(start), EndLine: int(end)}
		if lines, ok := canonical.AsSlice(m["lines"]); ok {
			for _, ln := range lines {
				s, ok := canonical.AsString(ln)
				if !ok {
					return nil, skeinerr.New(skeinerr.Malformed, "hunk line must be string")
				}
				h.Lines = append(h.Lines, s)
			}
		}
		out = append(out, h)
	}
	return out, nil
}

func DetectPatchConflicts(operations []map[string]any, baselinePaths []string, pathMode string) ([]string, error) {
	var reasons []string
	baseline := make([]string, 0, len(baselinePaths))
	for _, p := range baselinePaths {
		np, err := object.NormalizePath(p)
		if err != nil {
			return nil, err
		}
		baseline = append(baseline, np)
	}
	var reservedOld, reservedNew []string
	taken := func(path string, pool []string) bool {
		for _, x := range pool {
			if object.PathsConflict(path, x, pathMode) {
				return true
			}
		}
		return false
	}
	live := append([]string{}, baseline...)
	for i, op := range operations {
		kind, _ := canonical.AsString(op["op"])
		oldP, newP, err := opPaths(op)
		if err != nil {
			return nil, err
		}
		switch kind {
		case "add":
			if taken(newP, live) || taken(newP, reservedNew) {
				reasons = append(reasons, "op "+itoa(i)+": add of existing or reserved path "+newP)
			}
			reservedNew = append(reservedNew, newP)
			live = append(live, newP)
		case "remove":
			if !taken(oldP, live) {
				reasons = append(reasons, "op "+itoa(i)+": remove of missing path "+oldP)
			} else {
				live = filterPath(live, oldP, pathMode)
			}
			reservedOld = append(reservedOld, oldP)
		case "rename":
			if pathMode == "case-insensitive" && object.ASCIICasefold(oldP) == object.ASCIICasefold(newP) && oldP != newP {
				reasons = append(reasons, "op "+itoa(i)+": case-only rename rejected in v1")
			}
			if !taken(oldP, live) {
				reasons = append(reasons, "op "+itoa(i)+": rename of missing path "+oldP)
			}
			if taken(newP, live) && !object.PathsConflict(oldP, newP, pathMode) {
				reasons = append(reasons, "op "+itoa(i)+": rename target exists "+newP)
			}
			if taken(oldP, reservedOld) || taken(newP, reservedNew) {
				reasons = append(reasons, "op "+itoa(i)+": rename path reserved")
			}
			reservedOld = append(reservedOld, oldP)
			reservedNew = append(reservedNew, newP)
			live = filterPath(live, oldP, pathMode)
			live = append(live, newP)
		case "edit":
			if !taken(oldP, live) {
				reasons = append(reasons, "op "+itoa(i)+": edit of missing or removed path "+oldP)
			}
			if hunksRaw, ok := op["hunks"]; ok {
				hunks, err := HunksFromAny(hunksRaw)
				if err != nil {
					return nil, err
				}
				conf, err := DetectHunkConflicts(hunks)
				if err != nil {
					return nil, err
				}
				if len(conf) > 0 {
					reasons = append(reasons, "op "+itoa(i)+": internal hunk conflict")
				}
			}
		case "replace":
			if !taken(newP, live) {
				reasons = append(reasons, "op "+itoa(i)+": replace of missing path "+newP)
			}
		}
	}
	return reasons, nil
}

func ComposeTree(baseline map[string]string, operations []map[string]any, blobs map[string][]byte, pathMode, defaultLineEnding string) (map[string]string, error) {
	paths := make([]string, 0, len(baseline))
	for p := range baseline {
		paths = append(paths, p)
	}
	reasons, err := DetectPatchConflicts(operations, paths, pathMode)
	if err != nil {
		return nil, err
	}
	if len(reasons) > 0 {
		return nil, skeinerr.New(skeinerr.Conflict, strings.Join(reasons, "; "))
	}
	tree := make(map[string]string, len(baseline))
	for k, v := range baseline {
		tree[k] = v
	}
	findKey := func(path string) (string, bool) {
		for p := range tree {
			if object.PathsConflict(p, path, pathMode) {
				return p, true
			}
		}
		return "", false
	}
	for _, op := range operations {
		kind, _ := canonical.AsString(op["op"])
		switch kind {
		case "add":
			path, err := object.NormalizePath(asString(op["path"]))
			if err != nil {
				return nil, err
			}
			tree[path] = asString(op["blob_id"])
		case "remove":
			path, err := object.NormalizePath(asString(op["path"]))
			if err != nil {
				return nil, err
			}
			key, ok := findKey(path)
			if !ok {
				return nil, skeinerr.New(skeinerr.Conflict, "remove of missing path")
			}
			delete(tree, key)
		case "rename":
			oldP, err := object.NormalizePath(asString(op["from"]))
			if err != nil {
				return nil, err
			}
			newP, err := object.NormalizePath(asString(op["to"]))
			if err != nil {
				return nil, err
			}
			key, ok := findKey(oldP)
			if !ok {
				return nil, skeinerr.New(skeinerr.Conflict, "rename of missing path")
			}
			tree[newP] = tree[key]
			delete(tree, key)
		case "replace":
			path, err := object.NormalizePath(asString(op["path"]))
			if err != nil {
				return nil, err
			}
			key, ok := findKey(path)
			if !ok {
				return nil, skeinerr.New(skeinerr.Conflict, "replace of missing path")
			}
			tree[key] = asString(op["blob_id"])
		case "edit":
			path, err := object.NormalizePath(asString(op["path"]))
			if err != nil {
				return nil, err
			}
			key, ok := findKey(path)
			if !ok {
				return nil, skeinerr.New(skeinerr.Conflict, "edit of missing path")
			}
			raw, ok := blobs[tree[key]]
			if !ok {
				return nil, skeinerr.New(skeinerr.Malformed, "missing blob for edit")
			}
			text := string(raw)
			ending := defaultLineEnding
			if e, ok := canonical.AsString(op["line_ending"]); ok && e != "" {
				ending = e
			}
			hunks, err := HunksFromAny(op["hunks"])
			if err != nil {
				return nil, err
			}
			newText, err := ApplyHunks(text, ending, hunks)
			if err != nil {
				return nil, err
			}
			tree[key] = object.BlobID([]byte(newText))
		}
	}
	return tree, nil
}

func opPaths(op map[string]any) (oldP, newP string, err error) {
	kind, _ := canonical.AsString(op["op"])
	switch kind {
	case "add":
		newP, err = object.NormalizePath(asString(op["path"]))
		return "", newP, err
	case "remove":
		oldP, err = object.NormalizePath(asString(op["path"]))
		return oldP, "", err
	case "rename":
		oldP, err = object.NormalizePath(asString(op["from"]))
		if err != nil {
			return "", "", err
		}
		newP, err = object.NormalizePath(asString(op["to"]))
		return oldP, newP, err
	case "edit", "replace":
		p, err := object.NormalizePath(asString(op["path"]))
		return p, p, err
	default:
		return "", "", skeinerr.New(skeinerr.Malformed, "unknown patch op: "+kind)
	}
}

func filterPath(live []string, path, mode string) []string {
	out := live[:0]
	for _, p := range live {
		if !object.PathsConflict(p, path, mode) {
			out = append(out, p)
		}
	}
	return append([]string{}, out...)
}

func asString(v any) string {
	s, _ := canonical.AsString(v)
	return s
}

func itoa(i int) string {
	if i == 0 {
		return "0"
	}
	var b [12]byte
	n := len(b)
	neg := i < 0
	if neg {
		i = -i
	}
	for i > 0 {
		n--
		b[n] = byte('0' + i%10)
		i /= 10
	}
	if neg {
		n--
		b[n] = '-'
	}
	return string(b[n:])
}
