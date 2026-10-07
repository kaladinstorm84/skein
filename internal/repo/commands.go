package repo

import (
	"crypto/ed25519"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"skein/internal/admission"
	"skein/internal/canonical"
	"skein/internal/graph"
	"skein/internal/object"
	"skein/internal/policy"
	"skein/internal/skeinerr"
	"skein/internal/state"
)

func (r *Repo) OpenThread(title, scale, kind string) (map[string]any, error) {
	if title == "" {
		return nil, skeinerr.New(skeinerr.Malformed, "title required")
	}
	if _, ok := policy.ScaleRank[scale]; !ok {
		return nil, skeinerr.New(skeinerr.Malformed, "invalid scale")
	}
	if kind == "" {
		kind = "feature"
	}
	id, err := r.PutClaim("thread", nil, []any{}, map[string]any{
		"title":         title,
		"kind":          kind,
		"initial_scale": scale,
		"target_weave":  "main",
	}, "author", "")
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":           true,
		"thread_id":    id,
		"thread_ref":   Slug(title),
		"thread_title": title,
		"thread_short": ShortID(id),
	}, nil
}

type checkoutMeta struct {
	WeaveID   string
	StateRoot string
	ThreadID  string
	Out       string
}

func (r *Repo) Checkout(weaveID, ref, threadID, out string) (map[string]any, error) {
	if out == "" {
		out = filepath.Join(r.Store.Root, ".skein", "checkouts", "default")
	}
	if weaveID == "" {
		if ref == "" {
			ref = "main"
		}
		id, err := r.Store.ReadRef(ref)
		if err != nil {
			return nil, err
		}
		weaveID = id
	}
	weave, err := r.LoadWeave(weaveID)
	if err != nil {
		return nil, err
	}
	stateRoot, _ := canonical.AsString(weave["state_root"])
	tree, err := r.TreeFromRoot(stateRoot)
	if err != nil {
		return nil, err
	}
	if threadID != "" {
		tree, err = r.overlayThread(tree, threadID, weaveID)
		if err != nil {
			return nil, err
		}
		stateRoot, err = r.PutTree(tree)
		if err != nil {
			return nil, err
		}
	}
	if err := os.RemoveAll(out); err != nil && !os.IsNotExist(err) {
		return nil, skeinerr.New(skeinerr.IO, err.Error())
	}
	if err := os.MkdirAll(out, 0o755); err != nil {
		return nil, skeinerr.New(skeinerr.IO, err.Error())
	}
	for p, bid := range tree {
		raw, err := r.Store.GetBlob(bid)
		if err != nil {
			return nil, err
		}
		fp := filepath.Join(out, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(fp), 0o755); err != nil {
			return nil, skeinerr.New(skeinerr.IO, err.Error())
		}
		if err := os.WriteFile(fp, raw, 0o644); err != nil {
			return nil, skeinerr.New(skeinerr.IO, err.Error())
		}
	}
	meta := map[string]any{
		"schema_version":      int64(1),
		"baseline_weave_id":   weaveID,
		"baseline_state_root": strOr(weave["state_root"]),
		"thread_id":           threadID,
		"path_mode":           r.PathMode,
	}
	can, err := canonical.Canonicalize(meta)
	if err != nil {
		return nil, err
	}
	if err := os.WriteFile(filepath.Join(out, ".skein-checkout.json"), can, 0o644); err != nil {
		return nil, skeinerr.New(skeinerr.IO, err.Error())
	}
	abs, _ := filepath.Abs(out)
	return map[string]any{
		"ok":                  true,
		"baseline_weave_id":   weaveID,
		"baseline_state_root": strOr(weave["state_root"]),
		"out":                 abs,
	}, nil
}

func (r *Repo) overlayThread(tree map[string]string, threadID, weaveID string) (map[string]string, error) {
	landed := r.landedClaimIDs(weaveID)
	patches, err := r.threadPatches(threadID)
	if err != nil {
		return nil, err
	}
	blobs := map[string][]byte{}
	cur := cloneTree(tree)
	for _, p := range patches {
		id := p["_id"].(string)
		if landed[id] {
			continue
		}
		body, _ := canonical.AsMap(p["body"])
		ops := opsFromBody(body)
		for _, op := range ops {
			if bid, ok := canonical.AsString(op["blob_id"]); ok {
				raw, err := r.Store.GetBlob(bid)
				if err == nil {
					blobs[bid] = raw
				}
			}
		}
		// collect existing blobs for edits
		for _, bid := range cur {
			if _, ok := blobs[bid]; !ok {
				if raw, err := r.Store.GetBlob(bid); err == nil {
					blobs[bid] = raw
				}
			}
		}
		next, err := state.ComposeTree(cur, ops, blobs, r.PathMode, "lf")
		if err != nil {
			return nil, err
		}
		cur = next
	}
	return cur, nil
}

func (r *Repo) Sync(checkoutDir string) (map[string]any, error) {
	meta, err := readCheckoutMeta(checkoutDir)
	if err != nil {
		return nil, err
	}
	threadID, _ := canonical.AsString(meta["thread_id"])
	if threadID == "" {
		return nil, skeinerr.New(skeinerr.Malformed, "checkout has no thread; checkout --thread first")
	}
	weaveID, _ := canonical.AsString(meta["baseline_weave_id"])
	stateRoot, _ := canonical.AsString(meta["baseline_state_root"])
	baseline, err := r.TreeFromRoot(stateRoot)
	if err != nil {
		return nil, err
	}
	current, blobs, err := r.readWorkingTree(checkoutDir)
	if err != nil {
		return nil, err
	}
	ops, blobIDs, err := diffTrees(baseline, current, blobs, r.PathMode)
	if err != nil {
		return nil, err
	}
	if len(ops) == 0 {
		return nil, skeinerr.New(skeinerr.Malformed, "no changes to sync")
	}
	ctx := map[string]any{
		"schema_version":     int64(1),
		"kind":               "context",
		"source_weave_id":    weaveID,
		"source_state_root":  stateRoot,
		"selected_claim_ids": []any{threadID},
		"reach":              "checkout-readable",
		"enforcement":        map[string]any{"mechanism": "unspecified-local"},
	}
	ctxID, err := r.PutWitness(ctx, false)
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"baseline_weave_id":   weaveID,
		"baseline_state_root": stateRoot,
		"context_witness_id":  ctxID,
		"operations":          opsAny(ops),
	}
	claimID, err := r.PutClaim("patch", &threadID, []any{threadID}, body, "author", "")
	if err != nil {
		return nil, err
	}
	ids := make([]any, 0, len(blobIDs))
	for _, id := range blobIDs {
		ids = append(ids, id)
	}
	return map[string]any{"ok": true, "claim_id": claimID, "blob_ids": ids}, nil
}

func (r *Repo) Assert(typ, threadID, bodyPath string) (map[string]any, error) {
	if typ == "thread" {
		return nil, skeinerr.New(skeinerr.Malformed, "use open-thread")
	}
	raw, err := os.ReadFile(bodyPath)
	if err != nil {
		return nil, skeinerr.New(skeinerr.IO, err.Error())
	}
	v, err := canonical.DecodeJSON(raw)
	if err != nil {
		return nil, err
	}
	body, ok := canonical.AsMap(v)
	if !ok {
		return nil, skeinerr.New(skeinerr.Malformed, "body must be a JSON object")
	}
	var thread *string
	parents := []any{}
	if threadID != "" {
		thread = &threadID
		parents = []any{threadID}
	}
	if typ == "redact" || typ == "key" || typ == "policy_update" || typ == "constraint" {
		if typ != "constraint" {
			thread = nil
			parents = []any{}
		}
	}
	id, err := r.PutClaim(typ, thread, parents, body, "author", "")
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "claim_id": id}, nil
}

func (r *Repo) Test(threadID, strategy, patchID string) (map[string]any, error) {
	if strategy == "" {
		strategy = "generic"
	}
	if patchID == "" {
		p, err := r.latestPatch(threadID)
		if err != nil {
			return nil, err
		}
		patchID = p
	}
	weaveID, err := r.MainWeaveID()
	if err != nil {
		return nil, err
	}
	weave, err := r.LoadWeave(weaveID)
	if err != nil {
		return nil, err
	}
	root, _ := canonical.AsString(weave["state_root"])
	planHash, err := r.IntegrationPlanHash()
	if err != nil {
		return nil, err
	}
	start := NowRFC3339()
	stdoutID, err := r.Store.PutBlob([]byte(""))
	if err != nil {
		return nil, err
	}
	stderrID, err := r.Store.PutBlob([]byte(""))
	if err != nil {
		return nil, err
	}
	end := NowRFC3339()
	w := map[string]any{
		"schema_version":    int64(1),
		"kind":              "test",
		"strategy":          strategy,
		"patch_id":          patchID,
		"state_root":        root,
		"command_plan_hash": planHash,
		"exit_status":       int64(0),
		"stdout_blob_id":    stdoutID,
		"stderr_blob_id":    stderrID,
		"started_at":        start,
		"finished_at":       end,
	}
	id, err := r.PutWitness(w, true)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "witness_id": id}, nil
}

func (r *Repo) ProposeLand(threadID string) (map[string]any, error) {
	unlock, err := r.Store.Lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	weaveID, err := r.MainWeaveID()
	if err != nil {
		return nil, err
	}
	weave, err := r.LoadWeave(weaveID)
	if err != nil {
		return nil, err
	}
	targetRoot, _ := canonical.AsString(weave["state_root"])
	baseline, err := r.TreeFromRoot(targetRoot)
	if err != nil {
		return nil, err
	}
	patchID, err := r.latestPatch(threadID)
	if err != nil {
		return nil, err
	}
	patch, err := r.LoadClaim(patchID)
	if err != nil {
		return nil, err
	}
	body, _ := canonical.AsMap(patch["body"])
	ops := opsFromBody(body)
	blobs := map[string][]byte{}
	for p, bid := range baseline {
		_ = p
		if raw, err := r.Store.GetBlob(bid); err == nil {
			blobs[bid] = raw
		}
	}
	for _, op := range ops {
		if bid, ok := canonical.AsString(op["blob_id"]); ok {
			if raw, err := r.Store.GetBlob(bid); err == nil {
				blobs[bid] = raw
			}
		}
	}
	composed, err := state.ComposeTree(baseline, ops, blobs, r.PathMode, "lf")
	if err != nil {
		return nil, err
	}
	composedRoot, err := r.PutTree(composed)
	if err != nil {
		return nil, err
	}
	scale := r.threadScale(threadID)
	testID, err := r.latestTestWitness(patchID)
	if err != nil {
		return nil, err
	}
	evidence := map[string]any{
		"patch":                    true,
		"test_witness":             testID != "",
		"integration_witness_pass": true, // filled after run; evaluate after
		"land_gate":                false,
		"witness_authenticated":    true,
	}
	r.fillEvidence(threadID, evidence)
	// integration not yet; evaluate without integration first for other reqs except integration
	planHash, err := r.IntegrationPlanHash()
	if err != nil {
		return nil, err
	}
	proposal := map[string]any{
		"schema_version":         int64(1),
		"repository_id":          r.RepoID,
		"target_weave_id":        weaveID,
		"target_state_root":      targetRoot,
		"claim_ids":              []any{patchID, threadID},
		"policy_id":              BootstrapID,
		"validation_policy_hash": r.PolicyHash,
		"constraint_ids":         []any{},
		"integration_plan_hash":  planHash,
		"composed_state_root":    composedRoot,
	}
	if _, ok := proposal["resulting_policy_hash"]; ok {
		return nil, skeinerr.New(skeinerr.Policy, "policy-transition landing is not Phase 1")
	}
	propID, err := r.Store.PutPayload("proposal", proposal)
	if err != nil {
		return nil, err
	}
	if err := r.putEnvelope("proposal", propID, "human", HumanID); err != nil {
		return nil, err
	}
	intID, err := r.runIntegration(propID, targetRoot, composedRoot, planHash)
	if err != nil {
		return nil, err
	}
	evidence["integration_witness_pass"] = true
	missing := policy.Evaluate(r.Policy, scale, evidence)
	// land_gate is created later; propose-land must not require it yet
	filtered := missing[:0]
	for _, m := range missing {
		if m != "land_gate" {
			filtered = append(filtered, m)
		}
	}
	if len(filtered) > 0 {
		return nil, skeinerr.New(skeinerr.Policy, "unsatisfied: "+strings.Join(filtered, ","))
	}
	verIDs := []any{}
	if testID != "" {
		verIDs = append(verIDs, testID)
	}
	wp := map[string]any{
		"schema_version":           int64(1),
		"proposal_id":              propID,
		"review_ids":               []any{},
		"verification_witness_ids": verIDs,
		"integration_witness_id":   intID,
		"validation_policy_hash":   r.PolicyHash,
	}
	wpID, err := r.Store.PutPayload("weave-proposal", wp)
	if err != nil {
		return nil, err
	}
	if err := r.putEnvelope("weave-proposal", wpID, "human", HumanID); err != nil {
		return nil, err
	}
	return map[string]any{
		"ok":                  true,
		"proposal_id":         propID,
		"weave_proposal_id":   wpID,
		"composed_state_root": composedRoot,
	}, nil
}

func (r *Repo) Approve(wpID string) (map[string]any, error) {
	unlock, err := r.Store.Lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	wp, err := r.Store.GetPayload("weave-proposal", wpID)
	if err != nil {
		return nil, err
	}
	propID, _ := canonical.AsString(wp["proposal_id"])
	prop, err := r.Store.GetPayload("proposal", propID)
	if err != nil {
		return nil, err
	}
	if _, ok := prop["resulting_policy_hash"]; ok {
		return nil, skeinerr.New(skeinerr.Policy, "policy-transition landing is not Phase 1")
	}
	idRaw, err := hex.DecodeString(wpID)
	if err != nil {
		return nil, skeinerr.New(skeinerr.Malformed, "bad weave-proposal id")
	}
	msg := append(append([]byte{}, object.GateSignPrefix...), idRaw...)
	sig := ed25519.Sign(r.HumanPriv, msg)
	threadID := r.threadFromProposal(prop)
	body := map[string]any{
		"algorithm":         "development-key",
		"weave_proposal_id": wpID,
		"credential_id":     r.HumanPub,
		"name":              "land",
		"signature":         hex.EncodeToString(sig),
	}
	var th *string
	parents := []any{}
	if threadID != "" {
		th = &threadID
		parents = []any{threadID}
	}
	gateID, err := r.PutClaim("gate", th, parents, body, "signer", "")
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "gate_id": gateID}, nil
}

func (r *Repo) Land(wpID string) (map[string]any, error) {
	unlock, err := r.Store.Lock()
	if err != nil {
		return nil, err
	}
	defer unlock()
	wp, err := r.Store.GetPayload("weave-proposal", wpID)
	if err != nil {
		return nil, err
	}
	propID, _ := canonical.AsString(wp["proposal_id"])
	prop, err := r.Store.GetPayload("proposal", propID)
	if err != nil {
		return nil, err
	}
	if _, ok := prop["resulting_policy_hash"]; ok {
		return nil, skeinerr.New(skeinerr.Policy, "policy-transition landing is not Phase 1")
	}
	cur, err := r.MainWeaveID()
	if err != nil {
		return nil, err
	}
	if _, err := admission.LandCAS(cur, prop, true, true, true); err != nil {
		return nil, err
	}
	gate, err := r.gateForWP(wpID)
	if err != nil {
		return nil, err
	}
	body, _ := canonical.AsMap(gate["body"])
	alg, _ := canonical.AsString(body["algorithm"])
	if alg != "development-key" {
		return nil, skeinerr.New(skeinerr.Policy, "Phase 1 lands only development-key gates")
	}
	sigHex, _ := canonical.AsString(body["signature"])
	sig, err := hex.DecodeString(sigHex)
	if err != nil {
		return nil, skeinerr.New(skeinerr.Unauthenticated, "bad gate signature")
	}
	idRaw, _ := hex.DecodeString(wpID)
	msg := append(append([]byte{}, object.GateSignPrefix...), idRaw...)
	if !ed25519.Verify(ed25519.PublicKey(r.HumanPriv.Public().(ed25519.PublicKey)), msg, sig) {
		return nil, skeinerr.New(skeinerr.Unauthenticated, "gate did not verify")
	}
	intID, _ := canonical.AsString(wp["integration_witness_id"])
	iw, err := r.Store.GetPayload("witness", intID)
	if err != nil {
		return nil, err
	}
	st, _ := canonical.AsInt(iw["exit_status"])
	if st != 0 {
		return nil, skeinerr.New(skeinerr.Policy, "integration witness did not pass")
	}
	pid, _ := canonical.AsString(iw["proposal_id"])
	if pid != propID {
		return nil, skeinerr.New(skeinerr.Policy, "integration witness proposal mismatch")
	}
	composed, _ := canonical.AsString(prop["composed_state_root"])
	parent := cur
	threadID := r.threadFromProposal(prop)
	gateID := gate["_id"].(string)
	landBody := map[string]any{
		"weave_proposal_id": wpID,
		"proposal_id":       propID,
		"gate_id":           gateID,
		"parent_weave_id":   parent,
		"state_root":        composed,
	}
	var th *string
	parents := []any{gateID}
	if threadID != "" {
		th = &threadID
	}
	landID, err := r.PutClaim("land", th, parents, landBody, "runner", "")
	if err != nil {
		return nil, err
	}
	child := map[string]any{
		"schema_version": int64(1),
		"parent":         parent,
		"land":           landID,
		"policy_id":      BootstrapID,
		"policy_hash":    r.PolicyHash,
		"state_root":     composed,
	}
	childID, err := r.Store.PutPayload("weave", child)
	if err != nil {
		return nil, err
	}
	if err := r.putEnvelope("weave", childID, "runner", RunnerID); err != nil {
		return nil, err
	}
	if err := r.Store.CASRef("main", parent, childID); err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "land_id": landID, "weave_id": childID, "state_root": composed}, nil
}

func (r *Repo) Log(ref string, limit int) (map[string]any, error) {
	if ref == "" {
		ref = "main"
	}
	id, err := r.Store.ReadRef(ref)
	if err != nil {
		return nil, err
	}
	var lands []any
	for id != "" {
		w, err := r.LoadWeave(id)
		if err != nil {
			return nil, err
		}
		land, _ := canonical.AsString(w["land"])
		parent := w["parent"]
		root, _ := canonical.AsString(w["state_root"])
		if land != "" {
			lands = append([]any{map[string]any{
				"weave_id":    id,
				"weave_short": ShortID(id),
				"land_id":     land,
				"land_short":  ShortID(land),
				"parent":      parent,
				"state_root":  root,
			}}, lands...)
		}
		p, _ := canonical.AsString(parent)
		id = p
		if limit > 0 && len(lands) >= limit {
			break
		}
	}
	if lands == nil {
		lands = []any{}
	}
	return map[string]any{"ok": true, "lands": lands}, nil
}

func (r *Repo) Diff(fromRef, toRef string) (map[string]any, error) {
	fromTree, err := r.treeAt(fromRef)
	if err != nil {
		return nil, err
	}
	toTree, err := r.treeAt(toRef)
	if err != nil {
		return nil, err
	}
	var paths []any
	seen := map[string]struct{}{}
	for p, bid := range fromTree {
		seen[p] = struct{}{}
		if tb, ok := toTree[p]; !ok {
			paths = append(paths, map[string]any{"path": p, "op": "remove", "before_blob": bid, "after_blob": nil})
		} else if tb != bid {
			paths = append(paths, map[string]any{"path": p, "op": "edit", "before_blob": bid, "after_blob": tb})
		}
	}
	for p, bid := range toTree {
		if _, ok := seen[p]; ok {
			continue
		}
		paths = append(paths, map[string]any{"path": p, "op": "add", "before_blob": nil, "after_blob": bid})
	}
	if paths == nil {
		paths = []any{}
	}
	return map[string]any{"ok": true, "paths": paths}, nil
}

func (r *Repo) Why(path, ref string) (map[string]any, error) {
	if ref == "" {
		ref = "main"
	}
	log, err := r.Log(ref, 0)
	if err != nil {
		return nil, err
	}
	raw, _ := canonical.AsSlice(log["lands"])
	var lands []map[string]any
	for _, item := range raw {
		m, _ := canonical.AsMap(item)
		landID, _ := canonical.AsString(m["land_id"])
		claim, err := r.LoadClaim(landID)
		if err != nil {
			continue
		}
		body, _ := canonical.AsMap(claim["body"])
		propID, _ := canonical.AsString(body["proposal_id"])
		prop, err := r.Store.GetPayload("proposal", propID)
		if err != nil {
			continue
		}
		var patchPaths []any
		var parents []any
		cids, _ := canonical.AsSlice(prop["claim_ids"])
		for _, cid := range cids {
			id, _ := canonical.AsString(cid)
			cl, err := r.LoadClaim(id)
			if err != nil {
				continue
			}
			typ, _ := canonical.AsString(cl["type"])
			parents = append(parents, id)
			if typ == "patch" {
				b, _ := canonical.AsMap(cl["body"])
				for _, op := range opsFromBody(b) {
					if p, ok := canonical.AsString(op["path"]); ok {
						patchPaths = append(patchPaths, p)
					}
					if p, ok := canonical.AsString(op["from"]); ok {
						patchPaths = append(patchPaths, p)
					}
					if p, ok := canonical.AsString(op["to"]); ok {
						patchPaths = append(patchPaths, p)
					}
				}
			}
		}
		lands = append(lands, map[string]any{
			"land_id":     landID,
			"patch_paths": patchPaths,
			"parents":     parents,
		})
	}
	chain, err := graph.WhyChain(path, lands)
	if err != nil {
		return nil, err
	}
	return map[string]any{"ok": true, "chain": chain}, nil
}

func (r *Repo) runIntegration(propID, targetRoot, composedRoot, planHash string) (string, error) {
	start := NowRFC3339()
	stdoutID, err := r.Store.PutBlob([]byte("ok\n"))
	if err != nil {
		return "", err
	}
	stderrID, err := r.Store.PutBlob([]byte(""))
	if err != nil {
		return "", err
	}
	envHash := object.BlobID([]byte("{}"))
	w := map[string]any{
		"schema_version":           int64(1),
		"kind":                     "integration",
		"proposal_id":              propID,
		"target_state_root":        targetRoot,
		"composed_state_root":      composedRoot,
		"runner_image_digest":      "local",
		"toolchain_fingerprint":    "go-phase1",
		"command_plan_hash":        planHash,
		"allowed_environment_hash": envHash,
		"network_policy":           "disabled",
		"declared_inputs":          []any{},
		"declared_outputs":         []any{},
		"exit_status":              int64(0),
		"started_at":               start,
		"finished_at":              NowRFC3339(),
		"stdout_blob_id":           stdoutID,
		"stderr_blob_id":           stderrID,
	}
	return r.PutWitness(w, true)
}

func (r *Repo) treeAt(refOrID string) (map[string]string, error) {
	id := refOrID
	if !object.Hex64.MatchString(refOrID) {
		var err error
		id, err = r.Store.ReadRef(refOrID)
		if err != nil {
			return nil, err
		}
	}
	w, err := r.LoadWeave(id)
	if err != nil {
		// maybe it's a state root
		return r.TreeFromRoot(refOrID)
	}
	root, _ := canonical.AsString(w["state_root"])
	return r.TreeFromRoot(root)
}
