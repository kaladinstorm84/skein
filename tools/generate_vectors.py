"""Generate normative test vectors from tools/skein.py."""

from __future__ import annotations

import json
import sys
from pathlib import Path

from cryptography.hazmat.primitives.asymmetric.ed25519 import Ed25519PrivateKey

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT / "tools"))

import skein  # noqa: E402

VECTORS = ROOT / "vectors"
DEV_SEED = b"\x42" * 32


def dump(suite: str, name: str, inp: dict, expected: dict) -> None:
    dest = VECTORS / suite / name
    dest.mkdir(parents=True, exist_ok=True)
    (dest / "in.json").write_text(json.dumps(inp, indent=2) + "\n", encoding="utf-8")
    (dest / "expected.json").write_text(json.dumps(expected, indent=2) + "\n", encoding="utf-8")


def main() -> None:
    VECTORS.mkdir(exist_ok=True)

    # --- encoding ---
    cases = [
        ("object_key_sort", {"b": 1, "a": 2}, '{"a":2,"b":1}'),
        ("nested_key_sort", {"z": {"b": 1, "a": 0}, "a": []}, '{"a":[],"z":{"a":0,"b":1}}'),
        ("nfc_compose", {"e\u0301": "x"}, None),  # key is e + combining acute -> NFC é
        ("array_order", [3, 1, 2], "[3,1,2]"),
        ("bool_null", {"t": True, "f": False, "n": None}, '{"f":false,"n":null,"t":true}'),
        ("zero", 0, "0"),
        ("negative", -2, "-2"),
        ("string_quote", 'a"b\\c', '"a\\"b\\\\c"'),
        ("control_tab", "a\tb", '"a\\tb"'),
    ]
    for name, value, expected_text in cases:
        if name == "nfc_compose":
            canonical = skein.canonicalize(value).decode("utf-8")
            dump("encoding", name, {"op": "canonicalize", "value": value}, {"canonical": canonical})
        else:
            got = skein.canonicalize(value).decode("utf-8")
            assert got == expected_text, (name, got, expected_text)
            dump("encoding", name, {"op": "canonicalize", "value": value}, {"canonical": expected_text})

    dump(
        "encoding",
        "reject_float",
        {"op": "canonicalize", "value": 1.5},
        {"error": "malformed"},
    )
    dump(
        "encoding",
        "reject_huge_int",
        {"op": "canonicalize", "value": 9007199254740992},
        {"error": "malformed"},
    )
    dump(
        "encoding",
        "timestamp_ok",
        {"op": "parse_rfc3339", "value": "2026-10-07T11:00:00.000Z"},
        {"value": "2026-10-07T11:00:00.000Z"},
    )
    dump(
        "encoding",
        "timestamp_reject",
        {"op": "parse_rfc3339", "value": "2026-10-07T11:00:00Z"},
        {"error": "malformed"},
    )

    # --- ids ---
    blob = skein.blob_id(b"hello")
    dump("ids", "blob_hello", {"op": "blob_id", "raw_utf8": "hello"}, {"id": blob})

    claim = {
        "schema_version": 1,
        "type": "thread",
        "parents": [],
        "accountable_human": "human:alice",
        "role": "author",
        "model": "",
        "body": {
            "title": "First thread",
            "kind": "feature",
            "initial_scale": "trivial",
            "target_weave": "main",
        },
    }
    claim_id = skein.object_id("claim", claim)
    dump("ids", "claim_thread", {"op": "object_id", "kind": "claim", "payload": claim}, {"id": claim_id})
    dump(
        "ids",
        "derived_id_excluded",
        {"op": "object_id", "kind": "claim", "payload": {**claim, "id": "00" * 32}},
        {"error": "malformed"},
    )

    for kind, payload in [
        ("witness", {"schema_version": 1, "kind": "review", "review_claim_id": "ab" * 32}),
        ("proposal", {
            "schema_version": 1,
            "repository_id": "aa" * 32,
            "target_weave_id": "bb" * 32,
            "target_state_root": "cc" * 32,
            "claim_ids": [],
            "policy_id": "skein.bootstrap.v1",
            "validation_policy_hash": "dd" * 32,
            "constraint_ids": [],
            "integration_plan_hash": "ee" * 32,
            "composed_state_root": "ff" * 32,
        }),
        ("weave-proposal", {
            "schema_version": 1,
            "proposal_id": "11" * 32,
            "review_ids": [],
            "verification_witness_ids": [],
            "integration_witness_id": "22" * 32,
            "validation_policy_hash": "dd" * 32,
        }),
        ("weave", {
            "schema_version": 1,
            "parent": None,
            "land": None,
            "policy_id": "skein.bootstrap.v1",
            "policy_hash": "dd" * 32,
            "state_root": skein.empty_tree_root(),
        }),
        ("tree", skein.empty_tree_payload()),
        ("policy", json.loads((ROOT / "schemas/policy/bootstrap.v1.json").read_text(encoding="utf-8"))),
        ("envelope", {
            "schema_version": 1,
            "object_kind": "claim",
            "object_id": "aa" * 32,
            "issuer_id": "human:alice",
            "issuer_class": "human",
            "auth_method": "development-key",
            "signature": "00",
            "issued_at": "2026-10-07T11:00:00.000Z",
        }),
    ]:
        dump(
            "ids",
            f"kind_{kind.replace('-', '_')}",
            {"op": "object_id", "kind": kind, "payload": payload},
            {"id": skein.object_id(kind, payload)},
        )

    empty_root = skein.empty_tree_root()
    dump("trees", "empty", {"op": "tree_root", "entries": []}, {"state_root": empty_root})

    hello_blob = skein.blob_id(b"hello\n")
    entries = [{"path": "b.txt", "kind": "file", "blob_id": hello_blob}, {"path": "a.txt", "kind": "file", "blob_id": hello_blob}]
    dump(
        "trees",
        "two_files_sorted",
        {"op": "tree_root", "entries": entries},
        {"state_root": skein.tree_root(entries)},
    )

    dump("paths", "ok_nested", {"op": "normalize_path", "path": "src/app.rs"}, {"path": "src/app.rs"})
    dump("paths", "reject_dotdot", {"op": "normalize_path", "path": "a/../b"}, {"error": "malformed"})
    dump("paths", "reject_absolute", {"op": "normalize_path", "path": "/a"}, {"error": "malformed"})
    dump("paths", "reject_backslash", {"op": "normalize_path", "path": "a\\b"}, {"error": "malformed"})

    dump(
        "paths",
        "glob_any_dir",
        {"op": "glob_match", "pattern": "*.md", "path": "docs/readme.md"},
        {"match": True},
    )
    dump(
        "paths",
        "glob_double_star",
        {"op": "glob_match", "pattern": "src/**/x.rs", "path": "src/a/b/x.rs"},
        {"match": True},
    )
    dump(
        "paths",
        "glob_miss",
        {"op": "glob_match", "pattern": "src/*.rs", "path": "src/a/x.rs"},
        {"match": False},
    )

    dump(
        "hunks",
        "intersecting_replace",
        {
            "op": "detect_hunk_conflicts",
            "hunks": [
                {"start_line": 0, "end_line": 3, "lines": ["a"]},
                {"start_line": 2, "end_line": 4, "lines": ["b"]},
            ],
        },
        {"conflicts": [[0, 1]]},
    )
    dump(
        "hunks",
        "insert_at_replace_boundary",
        {
            "op": "detect_hunk_conflicts",
            "hunks": [
                {"start_line": 1, "end_line": 3, "lines": ["a"]},
                {"start_line": 3, "end_line": 3, "lines": ["b"]},
            ],
        },
        {"conflicts": [[0, 1]]},
    )
    dump(
        "hunks",
        "disjoint_ok",
        {
            "op": "detect_hunk_conflicts",
            "hunks": [
                {"start_line": 0, "end_line": 1, "lines": ["a"]},
                {"start_line": 2, "end_line": 3, "lines": ["b"]},
            ],
        },
        {"conflicts": []},
    )
    dump(
        "hunks",
        "apply_descending",
        {
            "op": "apply_hunks",
            "text": "a\nb\nc\n",
            "line_ending": "lf",
            "hunks": [
                {"start_line": 0, "end_line": 1, "lines": ["A"]},
                {"start_line": 2, "end_line": 3, "lines": ["C"]},
            ],
        },
        {"text": skein.apply_hunks("a\nb\nc\n", "lf", [
            {"start_line": 0, "end_line": 1, "lines": ["A"]},
            {"start_line": 2, "end_line": 3, "lines": ["C"]},
        ])},
    )
    dump(
        "hunks",
        "logical_final_empty",
        {"op": "logical_lines", "text": "a\n", "line_ending": "lf"},
        {"lines": ["a", ""]},
    )
    dump(
        "hunks",
        "empty_file",
        {"op": "logical_lines", "text": "", "line_ending": "lf"},
        {"lines": []},
    )

    base_blob = skein.blob_id(b"one\n")
    baseline = {"keep.txt": base_blob, "gone.txt": base_blob}
    blobs = {base_blob: {"utf8": "one\n"}}
    dump(
        "hunks",
        "rename_and_remove_ok",
        {
            "op": "compose",
            "baseline": baseline,
            "blobs": blobs,
            "path_mode": "case-sensitive",
            "operations": [
                {"op": "rename", "from": "keep.txt", "to": "kept.txt"},
                {"op": "remove", "path": "gone.txt"},
            ],
        },
        {
            "tree": {"kept.txt": base_blob},
            "state_root": skein.tree_root([{"path": "kept.txt", "kind": "file", "blob_id": base_blob}]),
        },
    )
    dump(
        "hunks",
        "two_adds_same_path",
        {
            "op": "compose",
            "baseline": {},
            "blobs": {},
            "path_mode": "case-sensitive",
            "operations": [
                {"op": "add", "path": "a.txt", "blob_id": base_blob},
                {"op": "add", "path": "a.txt", "blob_id": base_blob},
            ],
        },
        {"error": "conflict"},
    )
    dump(
        "hunks",
        "edit_removed",
        {
            "op": "compose",
            "baseline": {"a.txt": base_blob},
            "blobs": blobs,
            "path_mode": "case-sensitive",
            "operations": [
                {"op": "remove", "path": "a.txt"},
                {"op": "edit", "path": "a.txt", "line_ending": "lf", "hunks": [{"start_line": 0, "end_line": 1, "lines": ["x"]}]},
            ],
        },
        {"error": "conflict"},
    )
    dump(
        "hunks",
        "case_only_rename_rejected",
        {
            "op": "compose",
            "baseline": {"Readme.txt": base_blob},
            "blobs": blobs,
            "path_mode": "case-insensitive",
            "operations": [{"op": "rename", "from": "Readme.txt", "to": "README.txt"}],
        },
        {"error": "conflict"},
    )

    policy = json.loads((ROOT / "schemas/policy/bootstrap.v1.json").read_text(encoding="utf-8"))
    policy_hash = skein.object_id("policy", policy)
    root_weave = {
        "schema_version": 1,
        "parent": None,
        "land": None,
        "policy_id": "skein.bootstrap.v1",
        "policy_hash": policy_hash,
        "state_root": empty_root,
    }
    root_id = skein.object_id("weave", root_weave)
    land_blob = skein.blob_id(b"fn main() {}\n")
    composed = {"src/main.rs": land_blob}
    composed_root = skein.tree_root([{"path": "src/main.rs", "kind": "file", "blob_id": land_blob}])
    child_weave = {
        "schema_version": 1,
        "parent": root_id,
        "land": "ab" * 32,
        "policy_id": "skein.bootstrap.v1",
        "policy_hash": policy_hash,
        "state_root": composed_root,
    }
    child_id = skein.object_id("weave", child_weave)
    dump(
        "weaves",
        "parent_linked_replay",
        {"op": "weave_replay", "weaves": [root_weave, child_weave]},
        {
            "ids": [root_id, child_id],
            "state_roots": [empty_root, composed_root],
        },
    )

    proposal = {
        "schema_version": 1,
        "repository_id": skein.blob_id(skein.canonicalize({
            "schema_version": 1,
            "path_mode": "case-sensitive",
            "hash_alg": "sha256",
        })),
        "target_weave_id": root_id,
        "target_state_root": empty_root,
        "claim_ids": [claim_id],
        "policy_id": "skein.bootstrap.v1",
        "validation_policy_hash": policy_hash,
        "constraint_ids": [],
        "integration_plan_hash": skein.object_id("blob", policy["integration_plan"]) if False else skein.blob_id(skein.canonicalize(policy["integration_plan"])),
        "composed_state_root": composed_root,
    }
    proposal_id = skein.object_id("proposal", proposal)
    wp = {
        "schema_version": 1,
        "proposal_id": proposal_id,
        "review_ids": [],
        "verification_witness_ids": [],
        "integration_witness_id": "33" * 32,
        "validation_policy_hash": policy_hash,
    }
    wp_id = skein.object_id("weave-proposal", wp)
    dump(
        "proposals",
        "construct",
        {"op": "proposal_ids", "proposal": proposal, "weave_proposal": wp},
        {"proposal_id": proposal_id, "weave_proposal_id": wp_id},
    )
    transition = dict(proposal)
    transition["resulting_policy_hash"] = policy_hash
    dump(
        "policy-transition",
        "object_valid",
        {"op": "object_id", "kind": "proposal", "payload": transition},
        {"id": skein.object_id("proposal", transition)},
    )

    dump(
        "staleness",
        "target_advanced",
        {
            "op": "land_cas",
            "current_weave_id": child_id,
            "proposal": {"target_weave_id": root_id},
            "policy_ok": True,
            "gate_ok": True,
            "integration_ok": True,
        },
        {"error": "stale"},
    )
    dump(
        "staleness",
        "fresh_lands",
        {
            "op": "land_cas",
            "current_weave_id": root_id,
            "proposal": {"target_weave_id": root_id},
            "policy_ok": True,
            "gate_ok": True,
            "integration_ok": True,
        },
        {"action": "land", "parent_weave_id": root_id},
    )
    dump(
        "staleness",
        "bad_policy",
        {
            "op": "land_cas",
            "current_weave_id": root_id,
            "proposal": {"target_weave_id": root_id},
            "policy_ok": False,
            "gate_ok": True,
            "integration_ok": True,
        },
        {"error": "policy"},
    )

    dump(
        "policy",
        "trivial_pass",
        {
            "op": "evaluate_policy",
            "scale": "trivial",
            "evidence": {
                "patch": True,
                "test_witness": True,
                "integration_witness_pass": True,
                "land_gate": True,
            },
        },
        {"missing": []},
    )
    dump(
        "policy",
        "standard_missing_review",
        {
            "op": "evaluate_policy",
            "scale": "standard",
            "evidence": {
                "patch": True,
                "test_witness": True,
                "integration_witness_pass": True,
                "land_gate": True,
                "story": True,
                "acceptance": True,
                "verification_witnesses": True,
            },
        },
        {"missing": ["independent_review"]},
    )
    dump(
        "policy",
        "unauthenticated_witness",
        {
            "op": "evaluate_policy",
            "scale": "trivial",
            "evidence": {
                "patch": True,
                "test_witness": True,
                "integration_witness_pass": True,
                "land_gate": True,
                "witness_authenticated": False,
            },
        },
        {"missing": ["authenticated_witness"]},
    )
    dump(
        "policy",
        "gate_reuse_refused",
        {
            "op": "evaluate_policy",
            "scale": "trivial",
            "evidence": {
                "patch": True,
                "test_witness": True,
                "integration_witness_pass": True,
                "land_gate": True,
                "gate_reuse_requested": True,
            },
        },
        {"missing": ["gate_reuse_refused"]},
    )
    dump(
        "policy",
        "scale_down_blocked",
        {
            "op": "effective_scale",
            "assertions": [
                {"scale": "standard", "issuer_class": "human", "valid": True},
                {"scale": "trivial", "issuer_class": "agent", "valid": True},
            ],
        },
        {"scale": "standard"},
    )
    dump(
        "policy",
        "human_scale_down",
        {
            "op": "effective_scale",
            "assertions": [
                {"scale": "standard", "issuer_class": "human", "valid": True},
                {"scale": "trivial", "issuer_class": "human", "valid": True, "authorized_down": True},
            ],
        },
        {"scale": "trivial"},
    )
    dump(
        "policy",
        "thread_landed",
        {"op": "thread_state", "flags": {"building": True, "landed": True}},
        {"state": "landed"},
    )

    key = Ed25519PrivateKey.from_private_bytes(DEV_SEED)
    pub = key.public_key().public_bytes_raw().hex()
    good_sig = key.sign(skein.GATE_SIGN_PREFIX + bytes.fromhex(wp_id)).hex()
    dump(
        "gates",
        "development_key_accept",
        {
            "op": "gate_verify",
            "algorithm": "development-key",
            "weave_proposal_id": wp_id,
            "signature": good_sig,
            "public_key": pub,
        },
        {"ok": True},
    )
    dump(
        "gates",
        "development_key_reject",
        {
            "op": "gate_verify",
            "algorithm": "development-key",
            "weave_proposal_id": wp_id,
            "signature": "00" * 64,
            "public_key": pub,
        },
        {"ok": False},
    )
    dump(
        "gates",
        "webauthn_shape_not_phase1_land",
        {
            "op": "gate_phase1_accepts_algorithm",
            "algorithm": "webauthn",
        },
        {"phase1_land": False, "must_parse": True},
    )

    dump(
        "why",
        "last_land_wins",
        {
            "op": "why",
            "path": "src/main.rs",
            "lands": [
                {"land_id": "aa" * 32, "patch_paths": ["src/main.rs"], "parents": ["story-1"]},
                {"land_id": "bb" * 32, "patch_paths": ["src/other.rs"], "parents": ["story-2"]},
                {"land_id": "cc" * 32, "patch_paths": ["src/main.rs"], "parents": ["story-3", "gate-3"]},
            ],
        },
        {
            "path": "src/main.rs",
            "land_id": "cc" * 32,
            "parents": ["story-3", "gate-3"],
        },
    )
    dump(
        "why",
        "unknown_path",
        {
            "op": "why",
            "path": "missing.txt",
            "lands": [{"land_id": "aa" * 32, "patch_paths": ["src/main.rs"], "parents": []}],
        },
        {"chain": None},
    )

    constants = {
        "empty_tree_root": empty_root,
        "bootstrap_policy_hash": policy_hash,
        "bootstrap_policy_id": "skein.bootstrap.v1",
        "development_key_seed_hex": DEV_SEED.hex(),
        "development_key_public_hex": pub,
        "root_weave_id": root_id,
        "example_weave_proposal_id": wp_id,
        "example_proposal_id": proposal_id,
        "integration_plan_hash": proposal["integration_plan_hash"],
    }
    (ROOT / "schemas" / "constants.json").write_text(
        json.dumps(constants, indent=2) + "\n", encoding="utf-8"
    )
    print("wrote vectors and schemas/constants.json")
    for k, v in constants.items():
        print(f"  {k}={v}")


if __name__ == "__main__":
    main()
