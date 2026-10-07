package repo

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"skein/internal/canonical"
	"skein/internal/object"
	"skein/internal/skeinerr"
	"skein/internal/store"
	"skein/schemas"
)

const (
	HumanID       = "human:local"
	RunnerID      = "runner:local"
	EmptyRoot     = "cef3d661e4dd7ddc29be947eb6d6a18f897df3c9cddaed4dab683653fdd8a0fb"
	BootstrapHash = "4ccde27acc309e97f3d3eb326a246166f3658da2e3b6ccc027c8eb3526b1a066"
	BootstrapID   = "skein.bootstrap.v1"
)

type Repo struct {
	Store      *store.Store
	PathMode   string
	RepoID     string
	Policy     map[string]any
	PolicyHash string
	HumanPriv  ed25519.PrivateKey
	RunnerPriv ed25519.PrivateKey
	HumanPub   string
	RunnerPub  string
}

func NowRFC3339() string {
	return time.Now().UTC().Format("2006-01-02T15:04:05.000Z")
}

func FindRoot(start string) (string, error) {
	dir, err := filepath.Abs(start)
	if err != nil {
		return "", skeinerr.New(skeinerr.IO, err.Error())
	}
	for {
		if st, err := os.Stat(filepath.Join(dir, ".skein")); err == nil && st.IsDir() {
			return dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", skeinerr.New(skeinerr.IO, "not a skein repository")
		}
		dir = parent
	}
}

func Open(root string) (*Repo, error) {
	s := &store.Store{Root: root}
	if !s.Exists() {
		return nil, skeinerr.New(skeinerr.IO, "not a skein repository")
	}
	desc, err := s.ReadJSON("repository.json")
	if err != nil {
		return nil, err
	}
	mode, _ := canonical.AsString(desc["path_mode"])
	can, err := canonical.Canonicalize(desc)
	if err != nil {
		return nil, err
	}
	r := &Repo{Store: s, PathMode: mode, RepoID: object.BlobID(can)}
	pol, err := canonical.DecodeJSON(schemas.BootstrapV1)
	if err != nil {
		return nil, err
	}
	r.Policy, _ = canonical.AsMap(pol)
	r.PolicyHash, err = object.ObjectID("policy", r.Policy)
	if err != nil {
		return nil, err
	}
	if err := r.loadKeys(); err != nil {
		return nil, err
	}
	return r, nil
}

func Init(root, pathMode string) (*Repo, map[string]any, error) {
	if pathMode == "" {
		pathMode = "case-sensitive"
	}
	if pathMode != "case-sensitive" && pathMode != "case-insensitive" {
		return nil, nil, skeinerr.New(skeinerr.Malformed, "path-mode must be case-sensitive or case-insensitive")
	}
	s := &store.Store{Root: root}
	if s.Exists() {
		return nil, nil, skeinerr.New(skeinerr.IO, ".skein already exists")
	}
	if err := s.MkdirAll(); err != nil {
		return nil, nil, err
	}
	desc := map[string]any{
		"schema_version": int64(1),
		"path_mode":      pathMode,
		"hash_alg":       "sha256",
	}
	can, err := canonical.Canonicalize(desc)
	if err != nil {
		return nil, nil, err
	}
	repoID := object.BlobID(can)
	if _, err := s.PutBlob(can); err != nil {
		return nil, nil, err
	}
	if err := s.WriteJSON("repository.json", desc); err != nil {
		return nil, nil, err
	}
	polVal, err := canonical.DecodeJSON(schemas.BootstrapV1)
	if err != nil {
		return nil, nil, err
	}
	policy, _ := canonical.AsMap(polVal)
	policyHash, err := s.PutPayload("policy", policy)
	if err != nil {
		return nil, nil, err
	}
	if policyHash != BootstrapHash {
		return nil, nil, skeinerr.New(skeinerr.Malformed, "bootstrap policy hash mismatch: "+policyHash)
	}
	if err := s.WriteJSON("policy.json", map[string]any{
		"policy_id":   BootstrapID,
		"policy_hash": policyHash,
	}); err != nil {
		return nil, nil, err
	}
	r := &Repo{Store: s, PathMode: pathMode, RepoID: repoID, Policy: policy, PolicyHash: policyHash}
	if err := r.createKeys(); err != nil {
		return nil, nil, err
	}
	empty, err := object.EmptyTreeRoot()
	if err != nil {
		return nil, nil, err
	}
	if empty != EmptyRoot {
		return nil, nil, skeinerr.New(skeinerr.Malformed, "empty tree root mismatch: "+empty)
	}
	weave := map[string]any{
		"schema_version": int64(1),
		"parent":         nil,
		"land":           nil,
		"policy_id":      BootstrapID,
		"policy_hash":    policyHash,
		"state_root":     empty,
	}
	weaveID, err := s.PutPayload("weave", weave)
	if err != nil {
		return nil, nil, err
	}
	if err := r.putEnvelope("weave", weaveID, "human", HumanID); err != nil {
		return nil, nil, err
	}
	if err := r.putEnvelope("policy", policyHash, "human", HumanID); err != nil {
		return nil, nil, err
	}
	if err := s.WriteJSON(filepath.Join("refs", "weaves", "main"), map[string]any{
		"schema_version": int64(1),
		"name":           "main",
		"weave_id":       weaveID,
	}); err != nil {
		return nil, nil, err
	}
	return r, map[string]any{
		"ok":            true,
		"repository_id": repoID,
		"root_weave_id": weaveID,
		"state_root":    empty,
		"policy_hash":   policyHash,
	}, nil
}

func (r *Repo) createKeys() error {
	_, human, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return skeinerr.New(skeinerr.IO, err.Error())
	}
	_, runner, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return skeinerr.New(skeinerr.IO, err.Error())
	}
	r.HumanPriv, r.RunnerPriv = human, runner
	r.HumanPub = hex.EncodeToString(human.Public().(ed25519.PublicKey))
	r.RunnerPub = hex.EncodeToString(runner.Public().(ed25519.PublicKey))
	dir := filepath.Join(r.Store.SkeinDir(), "keys")
	if err := os.WriteFile(filepath.Join(dir, "development.ed25519"), []byte(hex.EncodeToString(human)), 0o600); err != nil {
		return skeinerr.New(skeinerr.IO, err.Error())
	}
	if err := os.WriteFile(filepath.Join(dir, "runner.ed25519"), []byte(hex.EncodeToString(runner)), 0o600); err != nil {
		return skeinerr.New(skeinerr.IO, err.Error())
	}
	if err := os.WriteFile(filepath.Join(dir, "development.pub"), []byte(r.HumanPub), 0o644); err != nil {
		return skeinerr.New(skeinerr.IO, err.Error())
	}
	return os.WriteFile(filepath.Join(dir, "principal"), []byte(HumanID), 0o644)
}

func (r *Repo) loadKeys() error {
	dir := filepath.Join(r.Store.SkeinDir(), "keys")
	h, err := os.ReadFile(filepath.Join(dir, "development.ed25519"))
	if err != nil {
		return skeinerr.New(skeinerr.Unauthenticated, "missing development key")
	}
	hb, err := hex.DecodeString(strings.TrimSpace(string(h)))
	if err != nil || len(hb) != ed25519.PrivateKeySize {
		return skeinerr.New(skeinerr.Unauthenticated, "invalid development key")
	}
	u, err := os.ReadFile(filepath.Join(dir, "runner.ed25519"))
	if err != nil {
		return skeinerr.New(skeinerr.Unauthenticated, "missing runner key")
	}
	ub, err := hex.DecodeString(strings.TrimSpace(string(u)))
	if err != nil || len(ub) != ed25519.PrivateKeySize {
		return skeinerr.New(skeinerr.Unauthenticated, "invalid runner key")
	}
	r.HumanPriv, r.RunnerPriv = ed25519.PrivateKey(hb), ed25519.PrivateKey(ub)
	r.HumanPub = hex.EncodeToString(r.HumanPriv.Public().(ed25519.PublicKey))
	r.RunnerPub = hex.EncodeToString(r.RunnerPriv.Public().(ed25519.PublicKey))
	return nil
}

func (r *Repo) putEnvelope(kind, objectID, issuerClass, issuerID string) error {
	idRaw, err := hex.DecodeString(objectID)
	if err != nil {
		return skeinerr.New(skeinerr.Malformed, "bad object id")
	}
	priv := r.HumanPriv
	method := "development-key"
	if issuerClass == "runner" {
		priv = r.RunnerPriv
		method = "runner-mac"
	}
	msg := append(append([]byte{}, object.EnvelopeSignPrefix...), idRaw...)
	sig := ed25519.Sign(priv, msg)
	env := map[string]any{
		"schema_version": int64(1),
		"object_kind":    kind,
		"object_id":      objectID,
		"issuer_id":      issuerID,
		"issuer_class":   issuerClass,
		"auth_method":    method,
		"signature":      hex.EncodeToString(sig),
		"issued_at":      NowRFC3339(),
	}
	eid, err := r.Store.PutPayload("envelope", env)
	if err != nil {
		return err
	}
	return r.Store.IndexPutEnvelope(objectID, eid)
}

func (r *Repo) PutClaim(typ string, thread *string, parents []any, body map[string]any, role, model string) (string, error) {
	payload := map[string]any{
		"schema_version":    int64(1),
		"type":              typ,
		"parents":           parents,
		"accountable_human": HumanID,
		"role":              role,
		"model":             model,
		"body":              body,
	}
	if thread != nil {
		payload["thread"] = *thread
	}
	id, err := r.Store.PutPayload("claim", payload)
	if err != nil {
		return "", err
	}
	class := "human"
	issuer := HumanID
	if typ == "land" {
		class = "runner"
		issuer = RunnerID
	}
	if err := r.putEnvelope("claim", id, class, issuer); err != nil {
		return "", err
	}
	return id, nil
}

func (r *Repo) PutWitness(payload map[string]any, runner bool) (string, error) {
	id, err := r.Store.PutPayload("witness", payload)
	if err != nil {
		return "", err
	}
	class, issuer := "human", HumanID
	if runner {
		class, issuer = "runner", RunnerID
	}
	if err := r.putEnvelope("witness", id, class, issuer); err != nil {
		return "", err
	}
	return id, nil
}

func (r *Repo) IntegrationPlanHash() (string, error) {
	plan, _ := canonical.AsMap(r.Policy["integration_plan"])
	can, err := canonical.Canonicalize(plan)
	if err != nil {
		return "", err
	}
	return object.BlobID(can), nil
}

func (r *Repo) MainWeaveID() (string, error) {
	return r.Store.ReadRef("main")
}

func (r *Repo) LoadWeave(id string) (map[string]any, error) {
	return r.Store.GetPayload("weave", id)
}

func (r *Repo) LoadClaim(id string) (map[string]any, error) {
	return r.Store.GetPayload("claim", id)
}

func (r *Repo) TreeFromRoot(stateRoot string) (map[string]string, error) {
	if stateRoot == EmptyRoot {
		return map[string]string{}, nil
	}
	ids, err := r.Store.ListIDs("tree")
	if err != nil {
		return nil, err
	}
	for _, id := range ids {
		if id == stateRoot {
			payload, err := r.Store.GetPayload("tree", id)
			if err != nil {
				return nil, err
			}
			return treeMap(payload)
		}
	}
	// Reconstruct by hashing candidate trees is expensive; store trees on write.
	payload, err := r.Store.GetPayload("tree", stateRoot)
	if err != nil {
		return nil, err
	}
	return treeMap(payload)
}

func treeMap(payload map[string]any) (map[string]string, error) {
	out := map[string]string{}
	entries, _ := canonical.AsSlice(payload["entries"])
	for _, e := range entries {
		m, _ := canonical.AsMap(e)
		p, _ := canonical.AsString(m["path"])
		b, _ := canonical.AsString(m["blob_id"])
		out[p] = b
	}
	return out, nil
}

func (r *Repo) PutTree(tree map[string]string) (string, error) {
	entries := make([]map[string]string, 0, len(tree))
	var anyEntries []any
	for p, bid := range tree {
		entries = append(entries, map[string]string{"path": p, "kind": "file", "blob_id": bid})
	}
	root, err := object.TreeRoot(entries)
	if err != nil {
		return "", err
	}
	for _, e := range entries {
		anyEntries = append(anyEntries, map[string]any{"blob_id": e["blob_id"], "kind": "file", "path": e["path"]})
	}
	payload := map[string]any{"entries": anyEntries, "schema_version": int64(1)}
	id, err := r.Store.PutPayload("tree", payload)
	if err != nil {
		return "", err
	}
	if id != root {
		return "", skeinerr.New(skeinerr.Malformed, "tree id mismatch")
	}
	return id, nil
}
