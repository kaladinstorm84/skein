"""Skein Canonical JSON v1, object IDs, trees, patches, and related algorithms.

This module is the vector-runner implementation of the normative algorithms
in skein-spec-0.7. Word is authoritative for admission intent; this code must
match Appendix D. Sidecar schemas and vectors are authoritative for byte identity.
"""

from __future__ import annotations

import hashlib
import json
import re
import unicodedata
from typing import Any, Iterable, Mapping, Sequence

JSON = Any

INT_MIN = -9007199254740991  # -(2**53) + 1
INT_MAX = 9007199254740991  # (2**53) - 1

PREFIXES: dict[str, bytes] = {
    "claim": b"skein.claim.v1\0",
    "witness": b"skein.witness.v1\0",
    "proposal": b"skein.proposal.v1\0",
    "weave-proposal": b"skein.weave-proposal.v1\0",
    "blob": b"skein.blob.v1\0",
    "weave": b"skein.weave.v1\0",
    "tree": b"skein.tree.v1\0",
    "policy": b"skein.policy.v1\0",
    "envelope": b"skein.envelope.v1\0",
}

GATE_SIGN_PREFIX = b"skein.gate.v1\0"

SCALE_RANK = {"trivial": 0, "standard": 1, "deep": 2}

HEX64 = re.compile(r"^[0-9a-f]{64}$")
RFC3339_MS_Z = re.compile(r"^[0-9]{4}-[0-9]{2}-[0-9]{2}T[0-9]{2}:[0-9]{2}:[0-9]{2}\.[0-9]{3}Z$")
PATH_COMPONENT_FORBIDDEN = frozenset({"", ".", ".."})


class SkeinError(Exception):
    def __init__(self, code: str, message: str) -> None:
        super().__init__(message)
        self.code = code
        self.message = message


def nfc(s: str) -> str:
    return unicodedata.normalize("NFC", s)


def encode_json_string(s: str) -> bytes:
    s = nfc(s)
    out: list[str] = ['"']
    for ch in s:
        o = ord(ch)
        if ch == '"':
            out.append('\\"')
        elif ch == "\\":
            out.append("\\\\")
        elif o == 0x08:
            out.append("\\b")
        elif o == 0x09:
            out.append("\\t")
        elif o == 0x0A:
            out.append("\\n")
        elif o == 0x0C:
            out.append("\\f")
        elif o == 0x0D:
            out.append("\\r")
        elif o < 0x20:
            out.append(f"\\u{o:04x}")
        else:
            out.append(ch)
    out.append('"')
    return "".join(out).encode("utf-8")


def canonicalize(value: JSON) -> bytes:
    """Return Canonical JSON v1 bytes. Rejects floats, non-NFC-invalid, out-of-range ints."""
    if value is None:
        return b"null"
    if isinstance(value, bool):
        return b"true" if value else b"false"
    if isinstance(value, int) and not isinstance(value, bool):
        if value < INT_MIN or value > INT_MAX:
            raise SkeinError("malformed", f"integer out of IEEE-safe range: {value}")
        return str(value).encode("ascii")
    if isinstance(value, float):
        raise SkeinError("malformed", "floating-point values are forbidden")
    if isinstance(value, str):
        return encode_json_string(value)
    if isinstance(value, list):
        return b"[" + b",".join(canonicalize(item) for item in value) + b"]"
    if isinstance(value, dict):
        norm_items: list[tuple[str, JSON]] = []
        seen: set[str] = set()
        for key, item in value.items():
            if not isinstance(key, str):
                raise SkeinError("malformed", "object keys must be strings")
            nk = nfc(key)
            if nk in seen:
                raise SkeinError("malformed", f"duplicate object key after NFC: {nk}")
            seen.add(nk)
            norm_items.append((nk, item))
        norm_items.sort(key=lambda kv: tuple(ord(c) for c in kv[0]))
        inner = b",".join(
            encode_json_string(k) + b":" + canonicalize(v) for k, v in norm_items
        )
        return b"{" + inner + b"}"
    raise SkeinError("malformed", f"unsupported canonical JSON type: {type(value).__name__}")


def sha256(data: bytes) -> bytes:
    return hashlib.sha256(data).digest()


def hex_id(digest: bytes) -> str:
    return digest.hex()


def object_id(kind: str, payload: JSON) -> str:
    if kind not in PREFIXES:
        raise SkeinError("malformed", f"unknown hashed kind: {kind}")
    if isinstance(payload, dict) and "id" in payload:
        raise SkeinError("malformed", "derived object id must not appear in hashed payload")
    return hex_id(sha256(PREFIXES[kind] + canonicalize(payload)))


def blob_id(raw: bytes) -> str:
    return hex_id(sha256(PREFIXES["blob"] + raw))


def empty_tree_payload() -> dict[str, JSON]:
    return {"schema_version": 1, "entries": []}


def tree_root(entries: Sequence[Mapping[str, str]]) -> str:
    normalized: list[dict[str, str]] = []
    seen: set[str] = set()
    for entry in entries:
        path = normalize_path(entry["path"])
        kind = entry["kind"]
        blob = entry["blob_id"]
        if kind != "file":
            raise SkeinError("malformed", "v1 trees permit kind=file only")
        if not HEX64.match(blob):
            raise SkeinError("malformed", "blob_id must be 64 lowercase hex chars")
        if path in seen:
            raise SkeinError("malformed", f"duplicate tree path: {path}")
        seen.add(path)
        normalized.append({"blob_id": blob, "kind": "file", "path": path})
    normalized.sort(key=lambda e: tuple(ord(c) for c in e["path"]))
    payload = {"entries": normalized, "schema_version": 1}
    return object_id("tree", payload)


def empty_tree_root() -> str:
    return tree_root([])


def normalize_path(path: str) -> str:
    p = nfc(path)
    if "\\" in p or "\x00" in p:
        raise SkeinError("malformed", "path must not contain NUL or backslash")
    if p.startswith("/") or p.endswith("/"):
        raise SkeinError("malformed", "path must be relative with no leading or trailing slash")
    parts = p.split("/")
    for part in parts:
        if part in PATH_COMPONENT_FORBIDDEN:
            raise SkeinError("malformed", f"forbidden path component: {part!r}")
    return p


def ascii_casefold(s: str) -> str:
    out = []
    for ch in s:
        o = ord(ch)
        if 65 <= o <= 90:
            out.append(chr(o + 32))
        else:
            out.append(ch)
    return "".join(out)


def paths_conflict_under_mode(a: str, b: str, path_mode: str) -> bool:
    if a == b:
        return True
    if path_mode == "case-insensitive" and ascii_casefold(a) == ascii_casefold(b):
        return True
    return False


def logical_lines(text: str, line_ending: str) -> list[str]:
    sep = "\n" if line_ending == "lf" else "\r\n"
    if line_ending not in ("lf", "crlf"):
        raise SkeinError("malformed", "line_ending must be lf or crlf")
    if text == "":
        return []
    return text.split(sep)


def join_lines(lines: Sequence[str], line_ending: str) -> str:
    sep = "\n" if line_ending == "lf" else "\r\n"
    if line_ending not in ("lf", "crlf"):
        raise SkeinError("malformed", "line_ending must be lf or crlf")
    return sep.join(lines)


def hunks_conflict(a: Mapping[str, JSON], b: Mapping[str, JSON]) -> bool:
    s1, e1 = int(a["start_line"]), int(a["end_line"])
    s2, e2 = int(b["start_line"]), int(b["end_line"])
    if s1 > e1 or s2 > e2 or s1 < 0 or s2 < 0:
        raise SkeinError("malformed", "hunk range must satisfy 0 <= start <= end")
    insert1 = s1 == e1
    insert2 = s2 == e2
    if not insert1 and not insert2:
        return max(s1, s2) < min(e1, e2)
    if insert1 and insert2:
        return s1 == s2
    p = s1 if insert1 else s2
    s, e = (s2, e2) if insert1 else (s1, e1)
    return s <= p <= e


def detect_hunk_conflicts(hunks: Sequence[Mapping[str, JSON]]) -> list[tuple[int, int]]:
    conflicts: list[tuple[int, int]] = []
    for i in range(len(hunks)):
        for j in range(i + 1, len(hunks)):
            if hunks_conflict(hunks[i], hunks[j]):
                conflicts.append((i, j))
    return conflicts


def apply_hunks(text: str, line_ending: str, hunks: Sequence[Mapping[str, JSON]]) -> str:
    if detect_hunk_conflicts(hunks):
        raise SkeinError("conflict", "hunk ranges conflict")
    lines = logical_lines(text, line_ending)
    ordered = sorted(enumerate(hunks), key=lambda it: (int(it[1]["start_line"]), it[0]), reverse=True)
    for _, hunk in ordered:
        start, end = int(hunk["start_line"]), int(hunk["end_line"])
        if end > len(lines) or start > len(lines):
            raise SkeinError("malformed", "hunk range exceeds file line count")
        new_lines = [nfc(x) for x in hunk["lines"]]
        lines[start:end] = new_lines
    return join_lines(lines, line_ending)


def _op_paths(op: Mapping[str, JSON]) -> tuple[str | None, str | None]:
    kind = op["op"]
    if kind == "add":
        return None, normalize_path(op["path"])
    if kind == "remove":
        return normalize_path(op["path"]), None
    if kind == "rename":
        return normalize_path(op["from"]), normalize_path(op["to"])
    if kind in ("edit", "replace"):
        p = normalize_path(op["path"])
        return p, p
    raise SkeinError("malformed", f"unknown patch op: {kind}")


def detect_patch_conflicts(
    operations: Sequence[Mapping[str, JSON]],
    baseline_paths: Iterable[str],
    path_mode: str = "case-sensitive",
) -> list[str]:
    """Return human-readable conflict reasons. Empty means structurally composable."""
    reasons: list[str] = []
    baseline = [normalize_path(p) for p in baseline_paths]
    reserved_old: list[str] = []
    reserved_new: list[str] = []

    def taken(path: str, pool: Sequence[str]) -> bool:
        return any(paths_conflict_under_mode(path, x, path_mode) for x in pool)

    live = list(baseline)
    for i, op in enumerate(operations):
        old, new = _op_paths(op)
        kind = op["op"]
        if kind == "add":
            assert new is not None
            if taken(new, live) or taken(new, reserved_new):
                reasons.append(f"op {i}: add of existing or reserved path {new}")
            reserved_new.append(new)
            live.append(new)
        elif kind == "remove":
            assert old is not None
            if not taken(old, live):
                reasons.append(f"op {i}: remove of missing path {old}")
            else:
                live = [p for p in live if not paths_conflict_under_mode(p, old, path_mode)]
            reserved_old.append(old)
        elif kind == "rename":
            assert old is not None and new is not None
            if path_mode == "case-insensitive" and ascii_casefold(old) == ascii_casefold(new) and old != new:
                reasons.append(f"op {i}: case-only rename rejected in v1")
            if not taken(old, live):
                reasons.append(f"op {i}: rename of missing path {old}")
            if taken(new, live) and not paths_conflict_under_mode(old, new, path_mode):
                reasons.append(f"op {i}: rename target exists {new}")
            if taken(old, reserved_old) or taken(new, reserved_new):
                reasons.append(f"op {i}: rename path reserved")
            reserved_old.append(old)
            reserved_new.append(new)
            live = [p for p in live if not paths_conflict_under_mode(p, old, path_mode)]
            live.append(new)
        elif kind == "edit":
            assert old is not None
            if not taken(old, live):
                reasons.append(f"op {i}: edit of missing or removed path {old}")
            if "hunks" in op and detect_hunk_conflicts(op["hunks"]):
                reasons.append(f"op {i}: internal hunk conflict")
        elif kind == "replace":
            assert new is not None
            if not taken(new, live):
                reasons.append(f"op {i}: replace of missing path {new}")
    # two adds of same path already covered via reserved_new
    return reasons


def compose_tree(
    baseline: Mapping[str, str],
    operations: Sequence[Mapping[str, JSON]],
    blobs: Mapping[str, bytes],
    path_mode: str = "case-sensitive",
    default_line_ending: str = "lf",
) -> dict[str, str]:
    """Apply operations to path->blob_id map. Returns new path->blob_id."""
    reasons = detect_patch_conflicts(operations, baseline.keys(), path_mode)
    if reasons:
        raise SkeinError("conflict", "; ".join(reasons))
    tree = dict(baseline)
    for op in operations:
        kind = op["op"]
        if kind == "add":
            path = normalize_path(op["path"])
            tree[path] = op["blob_id"]
        elif kind == "remove":
            path = normalize_path(op["path"])
            key = next(p for p in list(tree) if paths_conflict_under_mode(p, path, path_mode))
            del tree[key]
        elif kind == "rename":
            old = normalize_path(op["from"])
            new = normalize_path(op["to"])
            key = next(p for p in list(tree) if paths_conflict_under_mode(p, old, path_mode))
            tree[new] = tree.pop(key)
        elif kind == "replace":
            path = normalize_path(op["path"])
            key = next(p for p in list(tree) if paths_conflict_under_mode(p, path, path_mode))
            tree[key] = op["blob_id"]
        elif kind == "edit":
            path = normalize_path(op["path"])
            key = next(p for p in list(tree) if paths_conflict_under_mode(p, path, path_mode))
            raw = blobs[tree[key]]
            try:
                text = raw.decode("utf-8")
            except UnicodeDecodeError as exc:
                raise SkeinError("malformed", "edit requires UTF-8 file content") from exc
            ending = op.get("line_ending", default_line_ending)
            new_text = apply_hunks(text, ending, op["hunks"])
            new_raw = new_text.encode("utf-8")
            tree[key] = blob_id(new_raw)
    return tree


def glob_match(pattern: str, path: str) -> bool:
    """skein.glob.v1: ** across segments, * within a segment, ? one non-slash char.

    A pattern without `/` may match in any directory. A pattern with `/` is
    matched from the repository root.
    """
    pat = nfc(pattern)
    pth = normalize_path(path) if path not in ("",) else path
    if "/" not in pat.strip("/"):
        # match against any suffix segment sequence
        segs = pth.split("/")
        for i in range(len(segs)):
            if _glob_match_from_root(pat, "/".join(segs[i:])):
                return True
        return False
    return _glob_match_from_root(pat, pth)


def _glob_match_from_root(pattern: str, path: str) -> bool:
    return _glob_recursive(pattern.split("/"), path.split("/"))


def _glob_recursive(pat_segs: list[str], path_segs: list[str]) -> bool:
    if not pat_segs:
        return not path_segs
    head, *rest = pat_segs
    if head == "**":
        if not rest:
            return True
        for i in range(len(path_segs) + 1):
            if _glob_recursive(rest, path_segs[i:]):
                return True
        return False
    if not path_segs:
        return False
    if _seg_match(head, path_segs[0]):
        return _glob_recursive(rest, path_segs[1:])
    return False


def _seg_match(pat: str, seg: str) -> bool:
    i = j = 0
    star = -1
    star_j = 0
    while j < len(seg):
        if i < len(pat) and pat[i] == "*":
            star = i
            star_j = j
            i += 1
            continue
        if i < len(pat) and (pat[i] == "?" or pat[i] == seg[j]):
            i += 1
            j += 1
            continue
        if star != -1:
            i = star + 1
            star_j += 1
            j = star_j
            continue
        return False
    while i < len(pat) and pat[i] == "*":
        i += 1
    return i == len(pat)


def constraint_applies(scope: Sequence[str], old_path: str | None, new_path: str | None) -> bool:
    for pattern in scope:
        for candidate in (old_path, new_path):
            if candidate is not None and glob_match(pattern, candidate):
                return True
    return False


def evaluate_policy(policy: Mapping[str, JSON], scale: str, evidence: Mapping[str, JSON]) -> list[str]:
    """Return a list of unsatisfied requirement ids. Empty means pass."""
    if policy.get("gate_reuse") not in (None, "refuse"):
        # v1 object may carry the field; evaluation always refuses reuse.
        pass
    scales = policy["scales"]
    if scale not in scales:
        return ["unknown_scale"]
    required = scales[scale]
    missing: list[str] = []

    def need(flag: str, present: bool) -> None:
        if required.get(flag) and not present:
            missing.append(flag)

    need("patch", bool(evidence.get("patch")))
    need("test_witness", bool(evidence.get("test_witness")))
    need("integration_witness", bool(evidence.get("integration_witness_pass")))
    need("land_gate", bool(evidence.get("land_gate")))
    need("story", bool(evidence.get("story")))
    need("acceptance", bool(evidence.get("acceptance")))
    need("verification_witnesses", bool(evidence.get("verification_witnesses")))
    need("independent_review", bool(evidence.get("independent_review")))
    need("adversarial_witness", bool(evidence.get("adversarial_witness")))
    need("brief_or_requirement", bool(evidence.get("brief_or_requirement")))
    need("readiness_gate", bool(evidence.get("readiness_gate")))
    need("second_human_land_signer", bool(evidence.get("second_human_land_signer")))
    if evidence.get("witness_authenticated") is False:
        missing.append("authenticated_witness")
    if evidence.get("automated_scale_down"):
        missing.append("human_required_for_scale_down")
    if evidence.get("gate_reuse_requested"):
        missing.append("gate_reuse_refused")
    return missing


def derive_thread_state(flags: Mapping[str, bool]) -> str:
    """Priority order is normative. Abandoned and landed dominate later UI states."""
    if flags.get("abandoned"):
        return "abandoned"
    if flags.get("landed"):
        return "landed"
    if flags.get("gated"):
        return "gated"
    if flags.get("in_review"):
        return "in-review"
    if flags.get("building"):
        return "building"
    if flags.get("ready"):
        return "ready"
    return "draft"


def effective_scale(assertions: Sequence[Mapping[str, JSON]]) -> str:
    """Walk assertions in parent-graph order. Invalid items are ignored.

    Non-human issuers MAY raise scale and MUST NOT lower it. A human issuer
    MAY lower scale only when authorized_down is true under the active policy.
    """
    current = "trivial"
    for item in assertions:
        if not item.get("valid"):
            continue
        scale = item["scale"]
        if scale not in SCALE_RANK:
            continue
        rank = SCALE_RANK[scale]
        cur = SCALE_RANK[current]
        issuer = item.get("issuer_class", "human")
        if issuer != "human":
            if rank > cur:
                current = scale
            continue
        if rank < cur and not item.get("authorized_down"):
            continue
        current = scale
    return current


def land_cas(
    current_weave_id: str,
    proposal: Mapping[str, JSON],
    *,
    policy_ok: bool,
    gate_ok: bool,
    integration_ok: bool,
) -> dict[str, JSON]:
    if proposal.get("target_weave_id") != current_weave_id:
        raise SkeinError("stale", "target weave advanced; rebuild Proposal and WeaveProposal")
    if not policy_ok:
        raise SkeinError("policy", "pinned validation policy unsatisfied")
    if not integration_ok:
        raise SkeinError("policy", "integration witness did not pass for composed state")
    if not gate_ok:
        raise SkeinError("unauthenticated", "gate did not verify over WeaveProposal hash")
    return {"action": "land", "parent_weave_id": current_weave_id}


def why_chain(path: str, lands: Sequence[Mapping[str, JSON]]) -> dict[str, JSON] | None:
    """lands newest-last. Each land: {land_id, patch_paths, parents}."""
    target = normalize_path(path)
    for land in reversed(list(lands)):
        if target in land.get("patch_paths", []):
            return {
                "path": target,
                "land_id": land["land_id"],
                "parents": list(land.get("parents", [])),
            }
    return None


def parse_rfc3339(value: str) -> str:
    if not RFC3339_MS_Z.match(value):
        raise SkeinError("malformed", "timestamp must be RFC 3339 UTC with millisecond precision and Z")
    return value


def load_json(text: str) -> JSON:
    return json.loads(text)
