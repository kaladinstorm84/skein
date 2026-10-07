# Phase 1 command and agent ABI

**Normative** for the Phase 1 profile. Human surfaces (board, packet HTML) are not part of this ABI.

## 0 Errors

Every command writes a JSON object to stdout. On failure, `ok` is `false` and `error` is one of:

| Code | Meaning |
| --- | --- |
| `malformed` | Schema, Canonical JSON, path, or type error |
| `conflict` | Mechanical patch/hunk/path conflict |
| `stale` | Target weave, proposal, review, or witness no longer current |
| `unauthenticated` | Missing or invalid envelope or gate |
| `policy` | Bootstrap (or pinned) policy unsatisfied |
| `io` | Filesystem or lock failure |

```json
{"ok": false, "error": "stale", "message": "target weave advanced"}
```

Success:

```json
{"ok": true, ...}
```

Exit status is 0 if and only if `ok` is true. Implementations MUST NOT put business rules only in CLI flags; the same objects MUST be usable by the agent verbs in §2.

Stderr MAY contain diagnostics. It is not hashed and is not a conformance surface.

## 1 Commands

Working directory is the repository root unless `--dir` is given. `--json` is implied for Phase 1 conformance tests; pretty human text MAY also be printed to stderr.

### 1.1 `skein init`

```
skein init [--path-mode case-sensitive|case-insensitive]
```

Creates `.skein/`, repository descriptor, bootstrap policy reference (id `skein.bootstrap.v1`, hash `4ccde27acc309e97f3d3eb326a246166f3658da2e3b6ccc027c8eb3526b1a066`), local runner identity, development gate key, empty tree, root weave, and named ref `main`.

Success fields: `repository_id`, `root_weave_id`, `state_root` (MUST equal the published empty tree root), `policy_hash`.

### 1.2 `skein open-thread`

```
skein open-thread --title TEXT --scale trivial|standard|deep [--kind feature|fix|chore|policy|other]
```

Creates a `thread` claim. Success: `thread_id` (claim id), plus display fields `thread_ref` (title slug), `thread_title`, and `thread_short` (12-character unique prefix).

Object-id flags also accept a unique hex prefix. `--thread` also accepts the thread title or `thread_ref` slug. Full 64-character ids remain the hashed identity; short names are a CLI lookup layer only.

### 1.3 `skein checkout`

```
skein checkout [--weave WEAVE_ID | --ref main] [--thread THREAD_ID] [--out DIR]
```

Materializes a disposable checkout. Success: `baseline_weave_id`, `baseline_state_root`, `out`.

### 1.4 `skein sync`

```
skein sync --checkout DIR
```

Diffs the checkout against its recorded baseline and creates a `patch` claim. Success: `claim_id`, `blob_ids`. Failures include `conflict` if the working tree cannot be encoded under v1 rules.

### 1.5 `skein assert`

```
skein assert --type TYPE --thread THREAD_ID --body-file FILE.json
```

Creates a claim of the given type. Body MUST match the corresponding `schemas/claims/*.body.schema.json`. Success: `claim_id`.

### 1.6 `skein test`

```
skein test --thread THREAD_ID [--strategy generic|red-green] [--patch CLAIM_ID]
```

Runs a declared verification command plan and writes an authenticated `test` witness. Success: `witness_id`.

### 1.7 `skein propose-land`

```
skein propose-land --thread THREAD_ID
```

Evaluates policy, composes against the current `main` target, creates a Proposal, runs integration, emits a WeaveProposal. Success: `proposal_id`, `weave_proposal_id`, `composed_state_root`. May return `conflict` or `policy`.

### 1.8 `skein approve`

```
skein approve --weave-proposal ID
```

Creates a `gate` claim with `algorithm=development-key` over that WeaveProposal id. Success: `gate_id`. Failure: `unauthenticated` if the development key is missing.

### 1.9 `skein land`

```
skein land --weave-proposal ID
```

Re-reads `main`. If the target weave changed, fail `stale`. Otherwise revalidate policy, envelopes, witnesses, integration, WeaveProposal, and gate; write land and child weave; CAS `main`. Success: `land_id`, `weave_id`, `state_root`.

### 1.10 `skein log`

```
skein log [--ref main] [--limit N]
```

Success: `lands` array of `{weave_id, land_id, parent, state_root}` newest last.

### 1.11 `skein diff`

```
skein diff --from WEAVE_OR_STATE --to WEAVE_OR_STATE
```

File-level rendering. Success: `paths` array of `{path, op, before_blob, after_blob}`. Not an admission authority.

### 1.12 `skein why`

```
skein why --path PATH [--ref main]
```

Success: `chain` as in Appendix D.10, or `chain` null if the path was never landed.

### 1.13 `skein threads`

```
skein threads
```

Lists local thread claims. Success: `threads` as `{title, thread_ref, thread_short, thread_id, scale}`.

## 2 Agent verbs

JSON on stdin, JSON on stdout. Same error codes.

| Verb | stdin | stdout |
| --- | --- | --- |
| `open-thread` | `{title, scale, kind?}` | `{thread_id}` |
| `assert` | `{type, thread, body}` | `{claim_id}` |
| `project` | `{thread, reach}` | `{context_witness_id, bundle}` |
| `witness` | `{kind, thread, plan}` | `{witness_id}` |
| `sync` | `{checkout}` | `{claim_id}` |
| `propose-land` | `{thread}` | `{proposal_id, weave_proposal_id}` |
| `gate` | `{weave_proposal_id}` | `{gate_id}` |
| `land` | `{weave_proposal_id}` | `{land_id, weave_id}` |
| `query` | `{why_path? , log?}` | query-specific |
| `checkout` | `{weave?, thread?}` | `{out, baseline_weave_id}` |
| `port` | reserved | Phase 1 MUST return `error: policy` with message that port is not in Phase 1 |

`project` MUST write a `context` witness including selected claim ids, reach, and enforcement mechanism (Phase 1 MAY set `mechanism` to `unspecified-local`).

## 3 Locking

Phase 1 MUST take an exclusive lock on `.skein/lock` for `propose-land`, `approve`, and `land`. Lock failure is `io`.
