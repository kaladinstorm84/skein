package conformance

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"skein/internal/canonical"
)

func TestPublishedVectors(t *testing.T) {
	root, err := ModuleRoot()
	if err != nil {
		t.Fatal(err)
	}
	bootstrap, err := LoadBootstrap(root)
	if err != nil {
		t.Fatal(err)
	}
	var fixtures []string
	err = filepath.Walk(filepath.Join(root, "vectors"), func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.Name() == "in.json" {
			fixtures = append(fixtures, path)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(fixtures) == 0 {
		t.Fatal("no vectors")
	}
	failed := 0
	for _, inPath := range fixtures {
		raw, err := canonical.DecodeJSONFile(inPath)
		if err != nil {
			t.Errorf("decode %s: %v", inPath, err)
			failed++
			continue
		}
		inp, _ := canonical.AsMap(raw)
		got, err := RunOp(inp, bootstrap)
		if err != nil {
			t.Errorf("run %s: %v", inPath, err)
			failed++
			continue
		}
		expBytes, err := os.ReadFile(filepath.Join(filepath.Dir(inPath), "expected.json"))
		if err != nil {
			t.Fatal(err)
		}
		exp, err := canonical.DecodeJSON(expBytes)
		if err != nil {
			t.Errorf("expected %s: %v", inPath, err)
			failed++
			continue
		}
		gotN, err := roundtrip(got)
		if err != nil {
			t.Fatal(err)
		}
		rel, _ := filepath.Rel(filepath.Join(root, "vectors"), filepath.Dir(inPath))
		if !reflect.DeepEqual(gotN, exp) {
			t.Errorf("FAIL %s\n got: %#v\n exp: %#v", rel, gotN, exp)
			failed++
		}
	}
	if failed > 0 {
		t.Fatalf("%d vector fixtures failed", failed)
	}
}

func roundtrip(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	return canonical.DecodeJSON(b)
}

func TestEmptyTreeConstant(t *testing.T) {
	root, err := ModuleRoot()
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(filepath.Join(root, "schemas", "constants.json"))
	if !bytes.Contains(raw, []byte("cef3d661e4dd7ddc29be947eb6d6a18f897df3c9cddaed4dab683653fdd8a0fb")) {
		t.Fatal("empty tree root missing from constants")
	}
}
