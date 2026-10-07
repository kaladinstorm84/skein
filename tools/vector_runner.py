"""Execute published Skein vectors. Exit 0 iff all fixtures pass."""

from __future__ import annotations

import json
import sys
from pathlib import Path

from cryptography.exceptions import InvalidSignature
from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PublicKey
from jsonschema import Draft202012Validator
from jsonschema.exceptions import ValidationError
from referencing import Registry, Resource
from referencing.jsonschema import DRAFT202012

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tools"))

import skein  # noqa: E402

VECTORS = ROOT / "vectors"
SCHEMAS = ROOT / "schemas"


def load(path: Path):
    return json.loads(path.read_text(encoding="utf-8"))


def schema_registry() -> Registry:
    resources = []
    for path in SCHEMAS.rglob("*.schema.json"):
        data = load(path)
        sid = data.get("$id")
        if sid:
            resources.append((sid, Resource.from_contents(data, default_specification=DRAFT202012)))
    registry = Registry().with_resources(resources)
    return registry


REGISTRY = schema_registry()
POLICY = load(SCHEMAS / "policy" / "bootstrap.v1.json")


def schema_for_kind(kind: str) -> dict:
    index = load(SCHEMAS / "index.json")
    rel = index["hashed_kinds"].get(kind) or index["non_hashed"].get(kind)
    if not rel:
        raise SystemExit(f"no schema for kind {kind}")
    return load(SCHEMAS / rel)


def validate_schema(schema: dict, payload: dict) -> str | None:
    try:
        Draft202012Validator(schema, registry=REGISTRY).validate(payload)
    except ValidationError as exc:
        return str(exc.message)
    return None


def run_op(inp: dict) -> dict:
    op = inp["op"]
    try:
        if op == "canonicalize":
            return {"canonical": skein.canonicalize(inp["value"]).decode("utf-8")}
        if op == "parse_rfc3339":
            return {"value": skein.parse_rfc3339(inp["value"])}
        if op == "blob_id":
            return {"id": skein.blob_id(inp["raw_utf8"].encode("utf-8"))}
        if op == "object_id":
            return {"id": skein.object_id(inp["kind"], inp["payload"])}
        if op == "tree_root":
            return {"state_root": skein.tree_root(inp["entries"])}
        if op == "normalize_path":
            return {"path": skein.normalize_path(inp["path"])}
        if op == "glob_match":
            return {"match": skein.glob_match(inp["pattern"], inp["path"])}
        if op == "detect_hunk_conflicts":
            pairs = skein.detect_hunk_conflicts(inp["hunks"])
            return {"conflicts": [list(p) for p in pairs]}
        if op == "apply_hunks":
            return {"text": skein.apply_hunks(inp["text"], inp["line_ending"], inp["hunks"])}
        if op == "logical_lines":
            return {"lines": skein.logical_lines(inp["text"], inp["line_ending"])}
        if op == "compose":
            blobs = {}
            for bid, spec in inp.get("blobs", {}).items():
                blobs[bid] = spec["utf8"].encode("utf-8")
            tree = skein.compose_tree(
                inp["baseline"],
                inp["operations"],
                blobs,
                inp.get("path_mode", "case-sensitive"),
            )
            entries = [{"path": p, "kind": "file", "blob_id": b} for p, b in tree.items()]
            return {"tree": tree, "state_root": skein.tree_root(entries)}
        if op == "weave_replay":
            ids = [skein.object_id("weave", w) for w in inp["weaves"]]
            roots = [w["state_root"] for w in inp["weaves"]]
            return {"ids": ids, "state_roots": roots}
        if op == "proposal_ids":
            return {
                "proposal_id": skein.object_id("proposal", inp["proposal"]),
                "weave_proposal_id": skein.object_id("weave-proposal", inp["weave_proposal"]),
            }
        if op == "land_cas":
            result = skein.land_cas(
                inp["current_weave_id"],
                inp["proposal"],
                policy_ok=inp["policy_ok"],
                gate_ok=inp["gate_ok"],
                integration_ok=inp["integration_ok"],
            )
            return result
        if op == "evaluate_policy":
            missing = skein.evaluate_policy(POLICY, inp["scale"], inp["evidence"])
            return {"missing": missing}
        if op == "effective_scale":
            return {"scale": skein.effective_scale(inp["assertions"])}
        if op == "thread_state":
            return {"state": skein.derive_thread_state(inp["flags"])}
        if op == "gate_verify":
            if inp["algorithm"] != "development-key":
                return {"ok": False}
            pub = Ed25519PublicKey.from_public_bytes(bytes.fromhex(inp["public_key"]))
            try:
                pub.verify(
                    bytes.fromhex(inp["signature"]),
                    skein.GATE_SIGN_PREFIX + bytes.fromhex(inp["weave_proposal_id"]),
                )
                return {"ok": True}
            except (InvalidSignature, ValueError):
                return {"ok": False}
        if op == "gate_phase1_accepts_algorithm":
            alg = inp["algorithm"]
            return {"phase1_land": alg == "development-key", "must_parse": alg in ("development-key", "webauthn")}
        if op == "why":
            chain = skein.why_chain(inp["path"], inp["lands"])
            if chain is None:
                return {"chain": None}
            return chain
        if op == "schema_validate":
            schema = schema_for_kind(inp["kind"])
            err = validate_schema(schema, inp["payload"])
            if err:
                return {"error": "malformed", "detail": err}
            return {"ok": True}
        raise skein.SkeinError("malformed", f"unknown op {op}")
    except skein.SkeinError as exc:
        return {"error": exc.code}


def main() -> int:
    fixtures = sorted(VECTORS.glob("*/**/in.json"))
    if not fixtures:
        print("no vectors found", file=sys.stderr)
        return 1
    failed = 0
    for in_path in fixtures:
        exp_path = in_path.with_name("expected.json")
        inp = load(in_path)
        expected = load(exp_path)
        got = run_op(inp)
        label = str(in_path.parent.relative_to(VECTORS))
        if got != expected:
            failed += 1
            print(f"FAIL {label}")
            print(f"  got:      {json.dumps(got)}")
            print(f"  expected: {json.dumps(expected)}")
        else:
            print(f"PASS {label}")

    schema_checks = [
        ("policy", SCHEMAS / "policy.schema.json", POLICY),
        ("repository", SCHEMAS / "repository.schema.json", {
            "schema_version": 1,
            "path_mode": "case-sensitive",
            "hash_alg": "sha256",
        }),
        ("named-ref", SCHEMAS / "named-ref.schema.json", {
            "schema_version": 1,
            "name": "main",
            "weave_id": "aa" * 32,
        }),
        ("sync-packet", SCHEMAS / "sync-packet.schema.json", {
            "schema_version": 1,
            "kind": "have-want",
            "have_object_ids": [],
            "want_object_ids": [],
            "weave_pointers": {"main": "aa" * 32},
        }),
    ]
    for label, schema_path, payload in schema_checks:
        err = validate_schema(load(schema_path), payload)
        if err:
            print(f"FAIL schema/{label}: {err}")
            failed += 1
        else:
            print(f"PASS schema/{label}")

    kind_schema = {
        "claim": SCHEMAS / "claim.schema.json",
        "proposal": SCHEMAS / "proposal.schema.json",
        "weave-proposal": SCHEMAS / "weave-proposal.schema.json",
        "weave": SCHEMAS / "weave.schema.json",
        "tree": SCHEMAS / "tree.schema.json",
        "envelope": SCHEMAS / "envelope.schema.json",
        "witness": SCHEMAS / "witnesses" / "review.schema.json",
    }
    for in_path in VECTORS.glob("ids/*/in.json"):
        inp = load(in_path)
        if inp.get("op") != "object_id":
            continue
        kind = inp.get("kind")
        schema_path = kind_schema.get(kind)
        if not schema_path:
            continue
        if "id" in inp.get("payload", {}):
            continue
        err = validate_schema(load(schema_path), inp["payload"])
        label = f"schema/{in_path.parent.name}"
        if err:
            print(f"FAIL {label}: {err}")
            failed += 1
        else:
            print(f"PASS {label}")

    print(f"{len(fixtures) - failed}/{len(fixtures)} vector fixtures passed; failed={failed}")
    return 1 if failed else 0


if __name__ == "__main__":
    raise SystemExit(main())
