package repo

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"

	"skein/internal/canonical"
	"skein/internal/policy"
	"skein/internal/skeinerr"
	"skein/internal/state"
)

func strOr(v any) string {
	s, _ := canonical.AsString(v)
	return s
}

func cloneTree(t map[string]string) map[string]string {
	out := make(map[string]string, len(t))
	for k, v := range t {
		out[k] = v
	}
	return out
}

func opsFromBody(body map[string]any) []map[string]any {
	raw, _ := canonical.AsSlice(body["operations"])
	var ops []map[string]any
	for _, o := range raw {
		m, _ := canonical.AsMap(o)
		ops = append(ops, m)
	}
	return ops
}

func opsAny(ops []map[string]any) []any {
	out := make([]any, 0, len(ops))
	for _, o := range ops {
		out = append(out, o)
	}
	return out
}

func readCheckoutMeta(dir string) (map[string]any, error) {
	v, err := canonical.DecodeJSONFile(filepath.Join(dir, ".skein-checkout.json"))
	if err != nil {
		return nil, skeinerr.New(skeinerr.Malformed, "not a skein checkout")
	}
	m, _ := canonical.AsMap(v)
	return m, nil
}

func (r *Repo) readWorkingTree(dir string) (map[string]string, map[string][]byte, error) {
	tree := map[string]string{}
	blobs := map[string][]byte{}
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		if info.Name() == ".skein-checkout.json" {
			return nil
		}
		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		id, err := r.Store.PutBlob(raw)
		if err != nil {
			return err
		}
		tree[rel] = id
		blobs[id] = raw
		return nil
	})
	if err != nil {
		return nil, nil, skeinerr.New(skeinerr.IO, err.Error())
	}
	return tree, blobs, nil
}

func diffTrees(baseline, current map[string]string, blobs map[string][]byte, pathMode string) ([]map[string]any, []string, error) {
	var ops []map[string]any
	var blobIDs []string
	removed := map[string]string{} // path -> blob
	added := map[string]string{}
	for p, bid := range baseline {
		if _, ok := current[p]; !ok {
			removed[p] = bid
		} else if current[p] != bid {
			op, extra, err := fileChange(p, blobs[bid], blobs[current[p]], current[p])
			if err != nil {
				return nil, nil, err
			}
			ops = append(ops, op)
			blobIDs = append(blobIDs, extra...)
		}
	}
	for p, bid := range current {
		if _, ok := baseline[p]; !ok {
			added[p] = bid
		}
	}
	// greedy rename: same blob id
	for rp, rbid := range removed {
		found := ""
		for ap, abid := range added {
			if abid == rbid {
				found = ap
				break
			}
		}
		if found != "" {
			ops = append(ops, map[string]any{"op": "rename", "from": rp, "to": found})
			delete(added, found)
			delete(removed, rp)
		}
	}
	for p := range removed {
		ops = append(ops, map[string]any{"op": "remove", "path": p})
	}
	for p, bid := range added {
		ops = append(ops, map[string]any{"op": "add", "path": p, "blob_id": bid})
		blobIDs = append(blobIDs, bid)
	}
	_ = pathMode
	return ops, blobIDs, nil
}

func fileChange(path string, oldRaw, newRaw []byte, newBlob string) (map[string]any, []string, error) {
	if !utf8.Valid(oldRaw) || !utf8.Valid(newRaw) {
		return map[string]any{"op": "replace", "path": path, "blob_id": newBlob}, []string{newBlob}, nil
	}
	oldEnd := detectEnding(oldRaw)
	newEnd := detectEnding(newRaw)
	if oldEnd != newEnd {
		return map[string]any{"op": "replace", "path": path, "blob_id": newBlob}, []string{newBlob}, nil
	}
	oldText, newText := string(oldRaw), string(newRaw)
	oldL, err := state.LogicalLines(oldText, oldEnd)
	if err != nil {
		return nil, nil, err
	}
	newL, err := state.LogicalLines(newText, newEnd)
	if err != nil {
		return nil, nil, err
	}
	i := 0
	for i < len(oldL) && i < len(newL) && oldL[i] == newL[i] {
		i++
	}
	oEnd, nEnd := len(oldL), len(newL)
	for oEnd > i && nEnd > i && oldL[oEnd-1] == newL[nEnd-1] {
		oEnd--
		nEnd--
	}
	lines := make([]any, 0, nEnd-i)
	for _, l := range newL[i:nEnd] {
		lines = append(lines, l)
	}
	hunk := map[string]any{
		"start_line": int64(i),
		"end_line":   int64(oEnd),
		"lines":      lines,
	}
	return map[string]any{
		"op":          "edit",
		"path":        path,
		"line_ending": oldEnd,
		"hunks":       []any{hunk},
	}, nil, nil
}

func detectEnding(raw []byte) string {
	if strings.Contains(string(raw), "\r\n") {
		return "crlf"
	}
	return "lf"
}

func (r *Repo) claimsOfType(typ string) ([]map[string]any, error) {
	ids, err := r.Store.ListIDs("claim")
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for _, id := range ids {
		c, err := r.LoadClaim(id)
		if err != nil {
			continue
		}
		if strOr(c["type"]) != typ {
			continue
		}
		c["_id"] = id
		out = append(out, c)
	}
	return out, nil
}

func (r *Repo) threadPatches(threadID string) ([]map[string]any, error) {
	all, err := r.claimsOfType("patch")
	if err != nil {
		return nil, err
	}
	var out []map[string]any
	for _, c := range all {
		if strOr(c["thread"]) == threadID {
			out = append(out, c)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		return strOr(out[i]["_id"]) < strOr(out[j]["_id"])
	})
	return out, nil
}

func (r *Repo) latestPatch(threadID string) (string, error) {
	ps, err := r.threadPatches(threadID)
	if err != nil {
		return "", err
	}
	if len(ps) == 0 {
		return "", skeinerr.New(skeinerr.Malformed, "no patch on thread")
	}
	return strOr(ps[len(ps)-1]["_id"]), nil
}

func (r *Repo) latestTestWitness(patchID string) (string, error) {
	ids, err := r.Store.ListIDs("witness")
	if err != nil {
		return "", err
	}
	var found string
	for _, id := range ids {
		w, err := r.Store.GetPayload("witness", id)
		if err != nil {
			continue
		}
		if strOr(w["kind"]) == "test" && strOr(w["patch_id"]) == patchID {
			found = id
		}
	}
	return found, nil
}

func (r *Repo) threadScale(threadID string) string {
	c, err := r.LoadClaim(threadID)
	if err != nil {
		return "trivial"
	}
	body, _ := canonical.AsMap(c["body"])
	scale := strOr(body["initial_scale"])
	if scale == "" {
		scale = "trivial"
	}
	all, err := r.claimsOfType("scale_assertion")
	if err != nil {
		return scale
	}
	var assertions []map[string]any
	assertions = append(assertions, map[string]any{"scale": scale, "issuer_class": "human", "valid": true})
	for _, a := range all {
		if strOr(a["thread"]) != threadID {
			continue
		}
		b, _ := canonical.AsMap(a["body"])
		assertions = append(assertions, map[string]any{
			"scale":           strOr(b["scale"]),
			"issuer_class":    "human",
			"valid":           true,
			"authorized_down": b["authorized_down"],
		})
	}
	return policy.EffectiveScale(assertions)
}

func (r *Repo) fillEvidence(threadID string, evidence map[string]any) {
	all, err := r.claimsOfType("story")
	if err == nil {
		for _, c := range all {
			if strOr(c["thread"]) == threadID {
				evidence["story"] = true
			}
		}
	}
	acc, err := r.claimsOfType("acceptance")
	if err == nil {
		for _, c := range acc {
			if strOr(c["thread"]) == threadID {
				evidence["acceptance"] = true
			}
		}
	}
	rev, err := r.claimsOfType("review")
	if err == nil {
		for _, c := range rev {
			if strOr(c["thread"]) == threadID {
				evidence["independent_review"] = true
			}
		}
	}
	br, _ := r.claimsOfType("brief")
	rq, _ := r.claimsOfType("requirement")
	for _, c := range append(br, rq...) {
		if strOr(c["thread"]) == threadID {
			evidence["brief_or_requirement"] = true
		}
	}
	if test, _ := canonical.AsBool(evidence["test_witness"]); test {
		evidence["verification_witnesses"] = true
	}
}

func (r *Repo) landedClaimIDs(weaveID string) map[string]bool {
	out := map[string]bool{}
	id := weaveID
	for id != "" {
		w, err := r.LoadWeave(id)
		if err != nil {
			break
		}
		land := strOr(w["land"])
		if land != "" {
			c, err := r.LoadClaim(land)
			if err == nil {
				body, _ := canonical.AsMap(c["body"])
				propID := strOr(body["proposal_id"])
				if prop, err := r.Store.GetPayload("proposal", propID); err == nil {
					for _, cid := range mustSlice(prop["claim_ids"]) {
						out[cid] = true
					}
				}
			}
		}
		id = strOr(w["parent"])
	}
	return out
}

func mustSlice(v any) []string {
	raw, _ := canonical.AsSlice(v)
	var out []string
	for _, x := range raw {
		if s, ok := canonical.AsString(x); ok {
			out = append(out, s)
		}
	}
	return out
}

func (r *Repo) threadFromProposal(prop map[string]any) string {
	for _, id := range mustSlice(prop["claim_ids"]) {
		c, err := r.LoadClaim(id)
		if err != nil {
			continue
		}
		if t := strOr(c["thread"]); t != "" {
			return t
		}
		if strOr(c["type"]) == "thread" {
			// claim id is the thread
			return id
		}
	}
	return ""
}

func (r *Repo) gateForWP(wpID string) (map[string]any, error) {
	all, err := r.claimsOfType("gate")
	if err != nil {
		return nil, err
	}
	for i := len(all) - 1; i >= 0; i-- {
		b, _ := canonical.AsMap(all[i]["body"])
		if strOr(b["weave_proposal_id"]) == wpID {
			return all[i], nil
		}
	}
	return nil, skeinerr.New(skeinerr.Unauthenticated, "no gate for weave-proposal")
}
