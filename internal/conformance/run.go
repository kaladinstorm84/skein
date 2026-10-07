package conformance

import (
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"path/filepath"

	"skein/internal/admission"
	"skein/internal/canonical"
	"skein/internal/graph"
	"skein/internal/object"
	"skein/internal/policy"
	"skein/internal/skeinerr"
	"skein/internal/state"
)

func RunOp(inp map[string]any, bootstrap map[string]any) (map[string]any, error) {
	op, _ := canonical.AsString(inp["op"])
	switch op {
	case "canonicalize":
		b, err := canonical.Canonicalize(inp["value"])
		if err != nil {
			return errMap(err), nil
		}
		return map[string]any{"canonical": string(b)}, nil
	case "parse_rfc3339":
		v, _ := canonical.AsString(inp["value"])
		s, err := canonical.ParseRFC3339(v)
		if err != nil {
			return errMap(err), nil
		}
		return map[string]any{"value": s}, nil
	case "blob_id":
		s, _ := canonical.AsString(inp["raw_utf8"])
		return map[string]any{"id": object.BlobID([]byte(s))}, nil
	case "object_id":
		kind, _ := canonical.AsString(inp["kind"])
		id, err := object.ObjectID(kind, inp["payload"])
		if err != nil {
			return errMap(err), nil
		}
		return map[string]any{"id": id}, nil
	case "tree_root":
		entries, err := entriesFromAny(inp["entries"])
		if err != nil {
			return errMap(err), nil
		}
		root, err := object.TreeRoot(entries)
		if err != nil {
			return errMap(err), nil
		}
		return map[string]any{"state_root": root}, nil
	case "normalize_path":
		p, _ := canonical.AsString(inp["path"])
		np, err := object.NormalizePath(p)
		if err != nil {
			return errMap(err), nil
		}
		return map[string]any{"path": np}, nil
	case "glob_match":
		pat, _ := canonical.AsString(inp["pattern"])
		p, _ := canonical.AsString(inp["path"])
		ok, err := state.GlobMatch(pat, p)
		if err != nil {
			return errMap(err), nil
		}
		return map[string]any{"match": ok}, nil
	case "detect_hunk_conflicts":
		hunks, err := state.HunksFromAny(inp["hunks"])
		if err != nil {
			return errMap(err), nil
		}
		pairs, err := state.DetectHunkConflicts(hunks)
		if err != nil {
			return errMap(err), nil
		}
		out := make([]any, 0, len(pairs))
		for _, p := range pairs {
			out = append(out, []any{int64(p[0]), int64(p[1])})
		}
		return map[string]any{"conflicts": out}, nil
	case "apply_hunks":
		text, _ := canonical.AsString(inp["text"])
		ending, _ := canonical.AsString(inp["line_ending"])
		hunks, err := state.HunksFromAny(inp["hunks"])
		if err != nil {
			return errMap(err), nil
		}
		got, err := state.ApplyHunks(text, ending, hunks)
		if err != nil {
			return errMap(err), nil
		}
		return map[string]any{"text": got}, nil
	case "logical_lines":
		text, _ := canonical.AsString(inp["text"])
		ending, _ := canonical.AsString(inp["line_ending"])
		lines, err := state.LogicalLines(text, ending)
		if err != nil {
			return errMap(err), nil
		}
		arr := make([]any, len(lines))
		for i, l := range lines {
			arr[i] = l
		}
		return map[string]any{"lines": arr}, nil
	case "compose":
		return runCompose(inp)
	case "weave_replay":
		weaves, _ := canonical.AsSlice(inp["weaves"])
		ids := make([]any, 0, len(weaves))
		roots := make([]any, 0, len(weaves))
		for _, w := range weaves {
			m, _ := canonical.AsMap(w)
			id, err := object.ObjectID("weave", m)
			if err != nil {
				return errMap(err), nil
			}
			ids = append(ids, id)
			root, _ := canonical.AsString(m["state_root"])
			roots = append(roots, root)
		}
		return map[string]any{"ids": ids, "state_roots": roots}, nil
	case "proposal_ids":
		prop, _ := canonical.AsMap(inp["proposal"])
		wp, _ := canonical.AsMap(inp["weave_proposal"])
		pid, err := object.ObjectID("proposal", prop)
		if err != nil {
			return errMap(err), nil
		}
		wid, err := object.ObjectID("weave-proposal", wp)
		if err != nil {
			return errMap(err), nil
		}
		return map[string]any{"proposal_id": pid, "weave_proposal_id": wid}, nil
	case "land_cas":
		cur, _ := canonical.AsString(inp["current_weave_id"])
		prop, _ := canonical.AsMap(inp["proposal"])
		res, err := admission.LandCAS(cur, prop, truthy(inp["policy_ok"]), truthy(inp["gate_ok"]), truthy(inp["integration_ok"]))
		if err != nil {
			return errMap(err), nil
		}
		return res, nil
	case "evaluate_policy":
		scale, _ := canonical.AsString(inp["scale"])
		ev, _ := canonical.AsMap(inp["evidence"])
		missing := policy.Evaluate(bootstrap, scale, ev)
		arr := make([]any, len(missing))
		for i, m := range missing {
			arr[i] = m
		}
		return map[string]any{"missing": arr}, nil
	case "effective_scale":
		raw, _ := canonical.AsSlice(inp["assertions"])
		var assertions []map[string]any
		for _, a := range raw {
			m, _ := canonical.AsMap(a)
			assertions = append(assertions, m)
		}
		return map[string]any{"scale": policy.EffectiveScale(assertions)}, nil
	case "thread_state":
		flags, _ := canonical.AsMap(inp["flags"])
		return map[string]any{"state": policy.DeriveThreadState(flags)}, nil
	case "gate_verify":
		return runGateVerify(inp)
	case "gate_phase1_accepts_algorithm":
		alg, _ := canonical.AsString(inp["algorithm"])
		return map[string]any{
			"phase1_land": alg == "development-key",
			"must_parse":  alg == "development-key" || alg == "webauthn",
		}, nil
	case "why":
		path, _ := canonical.AsString(inp["path"])
		raw, _ := canonical.AsSlice(inp["lands"])
		var lands []map[string]any
		for _, l := range raw {
			m, _ := canonical.AsMap(l)
			lands = append(lands, m)
		}
		chain, err := graph.WhyChain(path, lands)
		if err != nil {
			return errMap(err), nil
		}
		if chain == nil {
			return map[string]any{"chain": nil}, nil
		}
		return chain, nil
	default:
		return map[string]any{"error": skeinerr.Malformed}, nil
	}
}

func runCompose(inp map[string]any) (map[string]any, error) {
	baseM, _ := canonical.AsMap(inp["baseline"])
	baseline := map[string]string{}
	for k, v := range baseM {
		s, _ := canonical.AsString(v)
		baseline[k] = s
	}
	blobSpec, _ := canonical.AsMap(inp["blobs"])
	blobs := map[string][]byte{}
	for id, spec := range blobSpec {
		m, _ := canonical.AsMap(spec)
		s, _ := canonical.AsString(m["utf8"])
		blobs[id] = []byte(s)
	}
	opsRaw, _ := canonical.AsSlice(inp["operations"])
	var ops []map[string]any
	for _, o := range opsRaw {
		m, _ := canonical.AsMap(o)
		ops = append(ops, m)
	}
	pathMode, _ := canonical.AsString(inp["path_mode"])
	if pathMode == "" {
		pathMode = "case-sensitive"
	}
	tree, err := state.ComposeTree(baseline, ops, blobs, pathMode, "lf")
	if err != nil {
		return errMap(err), nil
	}
	entries := make([]map[string]string, 0, len(tree))
	treeAny := map[string]any{}
	for p, bid := range tree {
		entries = append(entries, map[string]string{"path": p, "kind": "file", "blob_id": bid})
		treeAny[p] = bid
	}
	root, err := object.TreeRoot(entries)
	if err != nil {
		return errMap(err), nil
	}
	return map[string]any{"tree": treeAny, "state_root": root}, nil
}

func runGateVerify(inp map[string]any) (map[string]any, error) {
	alg, _ := canonical.AsString(inp["algorithm"])
	if alg != "development-key" {
		return map[string]any{"ok": false}, nil
	}
	pubHex, _ := canonical.AsString(inp["public_key"])
	sigHex, _ := canonical.AsString(inp["signature"])
	wp, _ := canonical.AsString(inp["weave_proposal_id"])
	pub, err := hex.DecodeString(pubHex)
	if err != nil || len(pub) != ed25519.PublicKeySize {
		return map[string]any{"ok": false}, nil
	}
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return map[string]any{"ok": false}, nil
	}
	idRaw, err := hex.DecodeString(wp)
	if err != nil {
		return map[string]any{"ok": false}, nil
	}
	msg := append(append([]byte{}, object.GateSignPrefix...), idRaw...)
	ok := ed25519.Verify(ed25519.PublicKey(pub), msg, sig)
	return map[string]any{"ok": ok}, nil
}

func entriesFromAny(v any) ([]map[string]string, error) {
	arr, _ := canonical.AsSlice(v)
	out := make([]map[string]string, 0, len(arr))
	for _, item := range arr {
		m, _ := canonical.AsMap(item)
		path, _ := canonical.AsString(m["path"])
		kind, _ := canonical.AsString(m["kind"])
		blob, _ := canonical.AsString(m["blob_id"])
		out = append(out, map[string]string{"path": path, "kind": kind, "blob_id": blob})
	}
	return out, nil
}

func errMap(err error) map[string]any {
	if e, ok := skeinerr.As(err); ok {
		return map[string]any{"error": e.Code}
	}
	return map[string]any{"error": skeinerr.Malformed}
}

func truthy(v any) bool {
	b, ok := canonical.AsBool(v)
	return ok && b
}

func LoadBootstrap(root string) (map[string]any, error) {
	v, err := canonical.DecodeJSONFile(filepath.Join(root, "schemas", "policy", "bootstrap.v1.json"))
	if err != nil {
		return nil, err
	}
	m, _ := canonical.AsMap(v)
	return m, nil
}

func ModuleRoot() (string, error) {
	dir, err := os.Getwd()
	if err != nil {
		return "", err
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", os.ErrNotExist
		}
		dir = parent
	}
}
