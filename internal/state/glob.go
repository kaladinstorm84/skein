package state

import (
	"strings"

	"skein/internal/canonical"
	"skein/internal/object"
)

func GlobMatch(pattern, path string) (bool, error) {
	pat := canonical.NFC(pattern)
	pth := path
	if path != "" {
		var err error
		pth, err = object.NormalizePath(path)
		if err != nil {
			return false, err
		}
	}
	if !strings.Contains(strings.Trim(pat, "/"), "/") {
		segs := strings.Split(pth, "/")
		for i := 0; i < len(segs); i++ {
			if globFromRoot(pat, strings.Join(segs[i:], "/")) {
				return true, nil
			}
		}
		return false, nil
	}
	return globFromRoot(pat, pth), nil
}

func globFromRoot(pattern, path string) bool {
	return globRecursive(strings.Split(pattern, "/"), strings.Split(path, "/"))
}

func globRecursive(patSegs, pathSegs []string) bool {
	if len(patSegs) == 0 {
		return len(pathSegs) == 0
	}
	head, rest := patSegs[0], patSegs[1:]
	if head == "**" {
		if len(rest) == 0 {
			return true
		}
		for i := 0; i <= len(pathSegs); i++ {
			if globRecursive(rest, pathSegs[i:]) {
				return true
			}
		}
		return false
	}
	if len(pathSegs) == 0 {
		return false
	}
	if segMatch(head, pathSegs[0]) {
		return globRecursive(rest, pathSegs[1:])
	}
	return false
}

func segMatch(pat, seg string) bool {
	i, j := 0, 0
	star, starJ := -1, 0
	for j < len(seg) {
		if i < len(pat) && pat[i] == '*' {
			star = i
			starJ = j
			i++
			continue
		}
		if i < len(pat) && (pat[i] == '?' || pat[i] == seg[j]) {
			i++
			j++
			continue
		}
		if star != -1 {
			i = star + 1
			starJ++
			j = starJ
			continue
		}
		return false
	}
	for i < len(pat) && pat[i] == '*' {
		i++
	}
	return i == len(pat)
}
