package abi

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"skein/internal/canonical"
	"skein/internal/repo"
)

func TestTrivialLandLoop(t *testing.T) {
	dir := t.TempDir()
	mustOK(t, runAt(dir, "init", "--path-mode", "case-sensitive"))
	th := mustOK(t, runAt(dir, "open-thread", "--title", "hello", "--scale", "trivial"))
	if th["thread_ref"] != "hello" || th["thread_short"] == nil {
		t.Fatalf("expected human thread_ref, got %#v", th)
	}
	outDir := filepath.Join(dir, "co")
	mustOK(t, runAt(dir, "checkout", "--thread", "hello", "--out", outDir))
	if err := os.WriteFile(filepath.Join(outDir, "hello.txt"), []byte("hello skein\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	sync := mustOK(t, runAt(dir, "sync", "--checkout", outDir))
	if _, ok := canonical.AsString(sync["claim_id"]); !ok {
		t.Fatalf("missing claim_id: %#v", sync)
	}
	mustOK(t, runAt(dir, "test", "--thread", "hello"))
	prop := mustOK(t, runAt(dir, "propose-land", "--thread", "hello"))
	wp, _ := canonical.AsString(prop["weave_proposal_id"])
	short, _ := canonical.AsString(prop["weave_proposal_id_short"])
	if wp == "" || short == "" {
		t.Fatalf("no weave-proposal: %#v", prop)
	}
	ap := mustOK(t, runAt(dir, "approve", "--weave-proposal", short))
	if _, ok := canonical.AsString(ap["gate_id"]); !ok {
		t.Fatalf("no gate: %#v", ap)
	}
	land := mustOK(t, runAt(dir, "land", "--weave-proposal", wp))
	if _, ok := canonical.AsString(land["land_id"]); !ok {
		t.Fatalf("no land: %#v", land)
	}
	if land["state_root"] == repo.EmptyRoot {
		t.Fatal("state root still empty after land")
	}
	stale := runAt(dir, "land", "--weave-proposal", wp)
	if stale["ok"] != false || stale["error"] != "stale" {
		t.Fatalf("expected stale reland, got %#v", stale)
	}
	why := mustOK(t, runAt(dir, "why", "--path", "hello.txt"))
	chain, _ := canonical.AsMap(why["chain"])
	if chain == nil {
		t.Fatalf("why chain missing: %#v", why)
	}
	lg := mustOK(t, runAt(dir, "log"))
	lands, _ := canonical.AsSlice(lg["lands"])
	if len(lands) != 1 {
		t.Fatalf("expected 1 land, got %#v", lg)
	}
	df := mustOK(t, runAt(dir, "diff", "--from", "main", "--to", "main"))
	if _, ok := df["paths"]; !ok {
		t.Fatalf("diff: %#v", df)
	}
	port := agentAt(dir, `{"verb":"port"}`)
	if port["ok"] != false || port["error"] != "policy" {
		t.Fatalf("port: %#v", port)
	}
}

func runAt(dir string, args ...string) map[string]any {
	all := append([]string{"--dir", dir}, args...)
	out, _ := Run(all, bytes.NewReader(nil))
	return out
}

func agentAt(dir, payload string) map[string]any {
	out, _ := Run([]string{"--dir", dir, "agent"}, strings.NewReader(payload))
	return out
}

func mustOK(t *testing.T, out map[string]any) map[string]any {
	t.Helper()
	if ok, _ := canonical.AsBool(out["ok"]); !ok {
		t.Fatalf("command failed: %#v", out)
	}
	return out
}
