package object

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"

	"skein/internal/canonical"
	"skein/internal/skeinerr"
)

var Hex64 = regexp.MustCompile(`^[0-9a-f]{64}$`)

var Prefixes = map[string][]byte{
	"claim":          []byte("skein.claim.v1\x00"),
	"witness":        []byte("skein.witness.v1\x00"),
	"proposal":       []byte("skein.proposal.v1\x00"),
	"weave-proposal": []byte("skein.weave-proposal.v1\x00"),
	"blob":           []byte("skein.blob.v1\x00"),
	"weave":          []byte("skein.weave.v1\x00"),
	"tree":           []byte("skein.tree.v1\x00"),
	"policy":         []byte("skein.policy.v1\x00"),
	"envelope":       []byte("skein.envelope.v1\x00"),
}

var GateSignPrefix = []byte("skein.gate.v1\x00")
var EnvelopeSignPrefix = []byte("skein.envelope.v1\x00")

func SHA256(data []byte) []byte {
	sum := sha256.Sum256(data)
	return sum[:]
}

func HexID(digest []byte) string {
	return hex.EncodeToString(digest)
}

func ObjectID(kind string, payload any) (string, error) {
	prefix, ok := Prefixes[kind]
	if !ok {
		return "", skeinerr.New(skeinerr.Malformed, "unknown hashed kind: "+kind)
	}
	if m, ok := payload.(map[string]any); ok {
		if _, has := m["id"]; has {
			return "", skeinerr.New(skeinerr.Malformed, "derived object id must not appear in hashed payload")
		}
	}
	can, err := canonical.Canonicalize(payload)
	if err != nil {
		return "", err
	}
	buf := make([]byte, 0, len(prefix)+len(can))
	buf = append(buf, prefix...)
	buf = append(buf, can...)
	return HexID(SHA256(buf)), nil
}

func BlobID(raw []byte) string {
	buf := make([]byte, 0, len(Prefixes["blob"])+len(raw))
	buf = append(buf, Prefixes["blob"]...)
	buf = append(buf, raw...)
	return HexID(SHA256(buf))
}

func EmptyTreeRoot() (string, error) {
	return TreeRoot(nil)
}

func TreeRoot(entries []map[string]string) (string, error) {
	normalized := make([]any, 0, len(entries))
	seen := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		path, err := NormalizePath(entry["path"])
		if err != nil {
			return "", err
		}
		kind := entry["kind"]
		blob := entry["blob_id"]
		if kind != "file" {
			return "", skeinerr.New(skeinerr.Malformed, "v1 trees permit kind=file only")
		}
		if !Hex64.MatchString(blob) {
			return "", skeinerr.New(skeinerr.Malformed, "blob_id must be 64 lowercase hex chars")
		}
		if _, ok := seen[path]; ok {
			return "", skeinerr.New(skeinerr.Malformed, "duplicate tree path: "+path)
		}
		seen[path] = struct{}{}
		normalized = append(normalized, map[string]any{
			"blob_id": blob,
			"kind":    "file",
			"path":    path,
		})
	}
	sortTreeEntries(normalized)
	payload := map[string]any{
		"entries":        normalized,
		"schema_version": int64(1),
	}
	return ObjectID("tree", payload)
}

func NormalizePath(path string) (string, error) {
	return normalizePath(path)
}
