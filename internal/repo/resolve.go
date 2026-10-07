package repo

import (
	"strings"
	"unicode"

	"skein/internal/canonical"
	"skein/internal/object"
	"skein/internal/skeinerr"
)

const ShortLen = 12

func ShortID(id string) string {
	if len(id) <= ShortLen {
		return id
	}
	return id[:ShortLen]
}

func Slug(title string) string {
	title = strings.ToLower(strings.TrimSpace(title))
	var b strings.Builder
	hyphen := false
	for _, r := range title {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
			hyphen = false
		default:
			if b.Len() > 0 && !hyphen {
				b.WriteByte('-')
				hyphen = true
			}
		}
	}
	return strings.Trim(b.String(), "-")
}

func (r *Repo) ResolveKind(kind, spec string) (string, error) {
	spec = strings.TrimSpace(strings.ToLower(spec))
	if spec == "" {
		return "", skeinerr.New(skeinerr.Malformed, "object id required")
	}
	if spec == "main" && kind == "weave" {
		return r.MainWeaveID()
	}
	if object.Hex64.MatchString(spec) {
		return spec, nil
	}
	ids, err := r.Store.ListIDs(kind)
	if err != nil {
		return "", err
	}
	var matches []string
	for _, id := range ids {
		if strings.HasPrefix(id, spec) {
			matches = append(matches, id)
		}
	}
	switch len(matches) {
	case 1:
		return matches[0], nil
	case 0:
		return "", skeinerr.New(skeinerr.Malformed, "no "+kind+" matches "+spec)
	default:
		return "", skeinerr.New(skeinerr.Malformed, "ambiguous "+kind+" prefix "+spec)
	}
}

func (r *Repo) ResolveThread(spec string) (string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" {
		return "", skeinerr.New(skeinerr.Malformed, "thread required")
	}
	low := strings.ToLower(spec)
	threads, err := r.claimsOfType("thread")
	if err != nil {
		return "", err
	}
	var byID, byName []string
	for _, c := range threads {
		id := strOr(c["_id"])
		body, _ := canonical.AsMap(c["body"])
		title := strOr(body["title"])
		slug := Slug(title)
		if id == low || strings.HasPrefix(id, low) {
			byID = append(byID, id)
		}
		if strings.EqualFold(title, spec) || slug == low {
			byName = append(byName, id)
		}
	}
	if len(byName) == 1 {
		return byName[0], nil
	}
	if len(byName) > 1 {
		return "", skeinerr.New(skeinerr.Malformed, "ambiguous thread name "+spec)
	}
	if len(byID) == 1 {
		return byID[0], nil
	}
	if len(byID) > 1 {
		return "", skeinerr.New(skeinerr.Malformed, "ambiguous thread prefix "+spec)
	}
	return "", skeinerr.New(skeinerr.Malformed, "no thread matches "+spec)
}

func (r *Repo) ThreadTitle(id string) string {
	c, err := r.LoadClaim(id)
	if err != nil {
		return ""
	}
	body, _ := canonical.AsMap(c["body"])
	return strOr(body["title"])
}

func (r *Repo) ListThreads() (map[string]any, error) {
	threads, err := r.claimsOfType("thread")
	if err != nil {
		return nil, err
	}
	items := make([]any, 0, len(threads))
	for _, c := range threads {
		id := strOr(c["_id"])
		body, _ := canonical.AsMap(c["body"])
		title := strOr(body["title"])
		items = append(items, map[string]any{
			"title":        title,
			"thread_ref":   Slug(title),
			"thread_short": ShortID(id),
			"thread_id":    id,
			"scale":        strOr(body["initial_scale"]),
		})
	}
	return map[string]any{"ok": true, "threads": items}, nil
}

func AnnotateHuman(out map[string]any) {
	if out == nil {
		return
	}
	keys := []string{
		"thread_id", "weave_proposal_id", "proposal_id", "gate_id",
		"land_id", "claim_id", "witness_id", "weave_id", "root_weave_id",
		"repository_id", "context_witness_id",
	}
	for _, k := range keys {
		id, ok := out[k].(string)
		if !ok || !object.Hex64.MatchString(id) {
			continue
		}
		out[k+"_short"] = ShortID(id)
	}
}

func (r *Repo) Annotate(out map[string]any) {
	AnnotateHuman(out)
	id, ok := out["thread_id"].(string)
	if !ok {
		return
	}
	title := r.ThreadTitle(id)
	if title == "" {
		return
	}
	out["thread_title"] = title
	out["thread_ref"] = Slug(title)
}
