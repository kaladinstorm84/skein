package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"skein/internal/canonical"
	"skein/internal/object"
	"skein/internal/skeinerr"
)

type Store struct {
	Root string
}

func (s *Store) SkeinDir() string { return filepath.Join(s.Root, ".skein") }

func (s *Store) objectPath(kind, id string) string {
	if len(id) < 2 {
		id = id + "00"
	}
	return filepath.Join(s.SkeinDir(), "objects", kind, id[:2], id)
}

func (s *Store) MkdirAll() error {
	dirs := []string{
		filepath.Join(s.SkeinDir(), "objects"),
		filepath.Join(s.SkeinDir(), "refs", "weaves"),
		filepath.Join(s.SkeinDir(), "keys"),
		filepath.Join(s.SkeinDir(), "index"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return skeinerr.New(skeinerr.IO, err.Error())
		}
	}
	return nil
}

func (s *Store) PutRaw(kind, id string, data []byte) error {
	p := s.objectPath(kind, id)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return skeinerr.New(skeinerr.IO, err.Error())
	}
	if err := os.WriteFile(p, data, 0o644); err != nil {
		return skeinerr.New(skeinerr.IO, err.Error())
	}
	return nil
}

func (s *Store) GetRaw(kind, id string) ([]byte, error) {
	b, err := os.ReadFile(s.objectPath(kind, id))
	if err != nil {
		return nil, skeinerr.New(skeinerr.Malformed, "missing object "+kind+"/"+id)
	}
	return b, nil
}

func (s *Store) PutPayload(kind string, payload any) (string, error) {
	id, err := object.ObjectID(kind, payload)
	if err != nil {
		return "", err
	}
	can, err := canonical.Canonicalize(payload)
	if err != nil {
		return "", err
	}
	if err := s.PutRaw(kind, id, can); err != nil {
		return "", err
	}
	return id, nil
}

func (s *Store) GetPayload(kind, id string) (map[string]any, error) {
	b, err := s.GetRaw(kind, id)
	if err != nil {
		return nil, err
	}
	v, err := canonical.DecodeJSON(b)
	if err != nil {
		return nil, err
	}
	m, ok := canonical.AsMap(v)
	if !ok {
		return nil, skeinerr.New(skeinerr.Malformed, "object is not a JSON object")
	}
	return m, nil
}

func (s *Store) PutBlob(raw []byte) (string, error) {
	id := object.BlobID(raw)
	return id, s.PutRaw("blob", id, raw)
}

func (s *Store) GetBlob(id string) ([]byte, error) {
	return s.GetRaw("blob", id)
}

func (s *Store) Exists() bool {
	st, err := os.Stat(s.SkeinDir())
	return err == nil && st.IsDir()
}

func (s *Store) WriteJSON(rel string, payload any) error {
	can, err := canonical.Canonicalize(payload)
	if err != nil {
		return err
	}
	p := filepath.Join(s.SkeinDir(), rel)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		return skeinerr.New(skeinerr.IO, err.Error())
	}
	return os.WriteFile(p, can, 0o644)
}

func (s *Store) ReadJSON(rel string) (map[string]any, error) {
	b, err := os.ReadFile(filepath.Join(s.SkeinDir(), rel))
	if err != nil {
		return nil, skeinerr.New(skeinerr.IO, err.Error())
	}
	v, err := canonical.DecodeJSON(b)
	if err != nil {
		return nil, err
	}
	m, ok := canonical.AsMap(v)
	if !ok {
		return nil, skeinerr.New(skeinerr.Malformed, "expected JSON object in "+rel)
	}
	return m, nil
}

func (s *Store) ListIDs(kind string) ([]string, error) {
	root := filepath.Join(s.SkeinDir(), "objects", kind)
	var ids []string
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			if os.IsNotExist(err) {
				return nil
			}
			return err
		}
		if info.IsDir() {
			return nil
		}
		id := info.Name()
		if object.Hex64.MatchString(id) {
			ids = append(ids, id)
		}
		return nil
	})
	if err != nil && !os.IsNotExist(err) {
		return nil, skeinerr.New(skeinerr.IO, err.Error())
	}
	return ids, nil
}

func (s *Store) Lock() (func() error, error) {
	p := filepath.Join(s.SkeinDir(), "lock")
	f, err := os.OpenFile(p, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return nil, skeinerr.New(skeinerr.IO, "could not acquire .skein/lock")
	}
	_, _ = f.WriteString(strings.TrimSpace(time.Now().UTC().Format(time.RFC3339Nano)))
	_ = f.Close()
	return func() error {
		return os.Remove(p)
	}, nil
}

func (s *Store) ReadRef(name string) (string, error) {
	m, err := s.ReadJSON(filepath.Join("refs", "weaves", name))
	if err != nil {
		return "", err
	}
	id, ok := canonical.AsString(m["weave_id"])
	if !ok {
		return "", skeinerr.New(skeinerr.Malformed, "named ref missing weave_id")
	}
	return id, nil
}

func (s *Store) CASRef(name, expected, next string) error {
	cur, err := s.ReadRef(name)
	if err != nil {
		return err
	}
	if cur != expected {
		return skeinerr.New(skeinerr.Stale, "named ref changed")
	}
	return s.WriteJSON(filepath.Join("refs", "weaves", name), map[string]any{
		"schema_version": int64(1),
		"name":           name,
		"weave_id":       next,
	})
}

func (s *Store) IndexPutEnvelope(objectID, envelopeID string) error {
	idx := map[string]any{}
	p := "index/envelopes.json"
	if m, err := s.ReadJSON(p); err == nil {
		idx = m
	}
	envelopes, _ := canonical.AsMap(idx["envelopes"])
	if envelopes == nil {
		envelopes = map[string]any{}
	}
	envelopes[objectID] = envelopeID
	idx["envelopes"] = envelopes
	// index is rebuildable; encoding/json is fine here (not hashed)
	b, err := json.Marshal(idx)
	if err != nil {
		return skeinerr.New(skeinerr.IO, err.Error())
	}
	full := filepath.Join(s.SkeinDir(), p)
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		return skeinerr.New(skeinerr.IO, err.Error())
	}
	return os.WriteFile(full, b, 0o644)
}

func (s *Store) EnvelopeIDFor(objectID string) (string, bool) {
	m, err := s.ReadJSON("index/envelopes.json")
	if err != nil {
		return "", false
	}
	envelopes, _ := canonical.AsMap(m["envelopes"])
	id, ok := canonical.AsString(envelopes[objectID])
	return id, ok
}
