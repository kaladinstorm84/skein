---
title: Skein
subtitle: A version control system for agentic development
---

# Skein

**Spec 0.7** | 7 October 2026 | Status: implementable Phase 1 specification

Skein is a version control system for AI-assisted development under the BMad method. It is not Git and is not a layer on Git. Its unit of history is a decision: a claim with explicit evidence, policy, and accountable human approval. Code is the materialized consequence of accepted decisions.

This document is the published Word narrative. Normative sidecar files live beside it in the specification repository. **On conflict, sidecar JSON Schema files and `vectors/` are authoritative for byte identity; this document is authoritative for admission intent and algorithms.** Every hashed field list in this document cites its schema file.

Authoring source for this Word file: `spec/skein-spec-0.7.md`. Regenerate with `python tools/build_spec.py`.

## 0 Conformance language and profiles

The key words MUST, MUST NOT, SHOULD, SHOULD NOT, and MAY are to be interpreted as described in RFC 2119.

Chapters 0–13, Appendix B, Appendix D, and Appendix E are **normative**. Appendix A, Appendix C, and human-surface descriptions are **informative** except where they repeat a MUST from a normative chapter.

Two conformance profiles exist:

- **Phase 1 profile.** Local kernel. MUST implement identity, patches, policy evaluation against the bootstrap pack, simulated `development-key` gates, and the command ABI in `abi/phase1.md`. MUST parse every v1 object shape. MUST NOT treat WebAuthn gates, remote sync, redaction, policy-transition landing, or projection-only sandboxes as trustworthy admission.
- **Full v1 object support.** Same object shapes. Additional behaviors (WebAuthn land gates, remote sync, redact application, policy-transition landing) MAY be implemented later without changing hashed payloads.

A Phase 1 implementation that also stores later-phase objects MUST refuse to execute those objects as if they were trustworthy.

## Changes from Draft 0.6

| Change | Resolution |
| --- | --- |
| RFC 2119 and sidecar authority | Added. Schemas and vectors win for bytes; Word wins for algorithms and admission. |
| Gate object | `algorithm` is `development-key` or `webauthn`. Phase 1 lands only `development-key`. |
| Weave definition | Parent-linked immutable manifest. Not a copied list of lands. |
| Section 8.2 | Duplicate “candidate hash” list removed. Gate signs the WeaveProposal id. |
| Canonical JSON | Frozen as SHA-256 only. IEEE-safe integers. Empty tree root published. |
| Object IDs | Prefixes added for weave, tree, policy, envelope. |
| Policy | Bootstrap pack `skein.bootstrap.v1` pinned by hash. Gate reuse field exists; v1 MUST refuse reuse. |
| Patch ops | add, remove, rename, edit, replace only. “Non-code claim-rendering” dropped from v1. |
| Open questions | Closed for v1 objects; remaining items are Phase 2/3 behavior. |

# 1 Purpose

Coding agents can produce patches quickly but do not by themselves preserve the judgment that made a patch safe to admit. BMad places that judgment in briefs, requirements, architecture constraints, stories, acceptance claims, tests, reviews, and human gates. Skein makes those artifacts first-class, content-addressed history rather than collateral files beside source code.

Skein stores planning artifacts, source content, and evidence in one append-only graph. A human approves a concrete WeaveProposal rather than a generic diff. A land is admitted only when its policy is satisfied and its composed result has passed integration.

## 1.1 Goals

- Store decisions, code, and evidence in a single content-addressed graph.
- Use a story and its acceptance claims as the unit of work rather than a commit message.
- Give agents bounded, reproducible projections and record the enforceable reach granted to them.
- Compose parallel work mechanically and require integration against the actual composed state.
- Keep human accountability separate from agent roles and model identity.
- Make gates cryptographically bind a human approval to an exact WeaveProposal.
- Retain abandoned, rejected, and retracted approaches as queryable evidence.

## 1.2 Non-goals

- Wire compatibility with Git packfiles, refs, or the Git index.
- Replacing compilers, test runners, editors, or language servers.
- Source-language parsing in the core; language adapters remain optional extensions.
- Proving what an LLM internally attended to or reasoned about.
- Encoding an entire development methodology; policy decides the required ceremony.

# 2 Terms

| Term | Meaning |
| --- | --- |
| Claim | An immutable, content-addressed assertion. The atom of history. Schema: `schemas/claim.schema.json`. |
| Thread | A named line of work opened by a `thread` claim. Visible status is derived, not a branch. |
| Weave | An immutable **parent-linked** manifest naming one admitted land (absent only at repository root), the policy in force, and the resulting state root. Ancestry orders lands. Schema: `schemas/weave.schema.json`. |
| Proposal | Pre-approval object: exact target weave and state, claims, validation policy, constraints, integration plan hash, composed state root. Schema: `schemas/proposal.schema.json`. |
| WeaveProposal | Human-approval object: Proposal id plus required reviews, verification witnesses, integration witness, and validation policy hash. Schema: `schemas/weave-proposal.schema.json`. |
| Gate | A human-authored **claim** of type `gate` with `algorithm` `development-key` or `webauthn` over a WeaveProposal id. Body schema: `schemas/claims/gate.body.schema.json`. |
| Witness | Content-addressed evidence. Schema: `schemas/witness.schema.json`. |
| Patch | A `patch` claim whose body is a deterministic list of file operations. Body schema: `schemas/claims/patch.body.schema.json`. |
| State root | Domain-separated hash of a canonical tree payload (kind `tree`). |
| Policy pack | Signed, versioned, machine-readable admissibility rules. Schema: `schemas/policy.schema.json`. |
| Envelope | Issuance authentication object. Not included in the payload id of the object it authenticates. Schema: `schemas/envelope.schema.json`. |

# 3 Architecture

Skein has three layers.

| Layer | Responsibility | Boundary |
| --- | --- | --- |
| Core | Object store, claims, threads, weave reconstruction, checkout, patch composition, policy evaluation, WeaveProposal validation. | Language-agnostic; files and bytes, not syntax. |
| Repository service | Remote sync, serialized land queue, weave compare-and-swap, human credential verification, packet and board surfaces. | Required for teams; MAY run in-process with a local lock for solo/Phase 1 use. |
| Extensions | Language adapters, projection retrieval, extra witness kinds, extra policy packs. | Out of process; conformance does not depend on them. |

# 4 Object identity and repository model

The repository is an append-only object store. Claims and witnesses are immutable. Supersession, abandonment, retraction, redaction, credential changes, and policy changes are new objects referring to prior objects. Derived indexes and user-interface status MAY be rebuilt from the object graph; they MUST NOT be an alternate source of truth.

## 4.1 Canonical encoding

Implementations MUST follow `schemas/canonical-json.md` (Skein Canonical JSON v1):

- UTF-8; Unicode NFC strings; no insignificant whitespace.
- Object keys sorted by Unicode code point of the NFC key, including nested objects.
- JSON integers only, in range −9007199254740991 … 9007199254740991. No floats.
- RFC 3339 UTC timestamps with millisecond precision and `Z` suffix when timestamps appear in hashed payloads.
- Explicit `schema_version` (const 1). Unknown fields in hashed v1 payloads MUST be rejected.
- Derived ids MUST NOT appear in hashed payloads.

Hash algorithm for v1 is SHA-256. Hash agility MUST NOT appear inside v1 objects. A later schema version MAY introduce a new domain prefix.

### Domain prefixes

| Kind | Prefix |
| --- | --- |
| claim | `skein.claim.v1\0` |
| witness | `skein.witness.v1\0` |
| proposal | `skein.proposal.v1\0` |
| weave-proposal | `skein.weave-proposal.v1\0` |
| blob | `skein.blob.v1\0` |
| weave | `skein.weave.v1\0` |
| tree | `skein.tree.v1\0` |
| policy | `skein.policy.v1\0` |
| envelope | `skein.envelope.v1\0` |

```
id = lowercase_hex( SHA-256( prefix || canonical(payload) ) )
blob_id = lowercase_hex( SHA-256( "skein.blob.v1\0" || raw_bytes ) )
```

A conforming implementation MUST pass `vectors/encoding` and `vectors/ids`.

## 4.2 Object taxonomy

| Object | Hashed kind | Claim? | Envelope | In a Proposal claim_ids |
| --- | --- | --- | --- | --- |
| Blob | blob | no | no | no (referenced by blob_id) |
| Claim | claim | yes | yes | yes |
| Witness | witness | no | yes | no (referenced by witness ids) |
| Proposal | proposal | no | yes | no |
| WeaveProposal | weave-proposal | no | yes | no |
| Weave manifest | weave | no | yes | no |
| Canonical tree | tree | no | no | no (state_root) |
| Policy pack | policy | no | yes | no |
| Issuance envelope | envelope | no | n/a | no |
| Named ref `main` | not hashed | no | no | no |
| Sync packet | not hashed | no | no | no |
| Repository descriptor | blob of Canonical JSON | no | no | no |

Repository identity is the blob id of the Canonical JSON encoding of `schemas/repository.schema.json`. Path mode is `case-sensitive` or `case-insensitive`. v1 case-insensitive comparison is **ASCII letter case-fold only** (A–Z to a–z).

## 4.3 Claim

Canonical payload: `schema_version`, `type`, `thread` (rules in Appendix E), `parents`, `accountable_human`, `role`, `model` (empty string if directly human-authored), `body`, optional `supersedes`. Schema: `schemas/claim.schema.json`. Body schemas: `schemas/claims/*.body.schema.json`. Catalog: `schemas/claims/catalog.json`.

A created timestamp belongs on the envelope, not the claim payload. Parents are semantic dependencies. Claims that omit required parents or fail their type schema are malformed (`error: malformed`).

Thread id is the claim id of the opening `thread` claim.

## 4.4 Thread derived state

Derived states: `draft`, `ready`, `building`, `in-review`, `gated`, `landed`, `abandoned`. Algorithm `derive_thread_state` (Appendix D) uses this priority: abandoned, landed, gated, in-review, building, ready, else draft.

`effective_scale` walks valid `scale_assertion` claims in parent-graph order (not wall-clock). Envelope `issued_at` is display-only. Automated issuers MUST NOT lower scale. A human MAY lower scale only with `authorized_down` and a policy-authorized verdict.

## 4.5 Weave and state root

A weave payload names `parent` (null only for the repository root), `land` (null only for the root), `policy_id`, `policy_hash`, and `state_root`. Named weave pointers such as `main` advance only by compare-and-swap (schema `schemas/named-ref.schema.json`).

A state root is `object_id("tree", {schema_version:1, entries:[...]} )`. Entries are `{path, kind, blob_id}` with `kind` equal to `file`, sorted by path code point. v1 MUST reject symlinks, executable bits, and submodules.

**Empty tree root** (published constant):

`cef3d661e4dd7ddc29be947eb6d6a18f897df3c9cddaed4dab683653fdd8a0fb`

A land is never removed from history. A retract is a human-authorized **forward** correction: it identifies the withdrawn land and contains explicit patch operations for the corrected state. Checkout is a deterministic rendering of a weave’s state root, not new history.

# 5 Claims and evidence

## 5.1 Constraint

A constraint body declares `scope` (skein.glob.v1 patterns; grammar in Appendix D), `check_kind` (`analyzer`, `contract-test`, `advisory`), `mandatory`, and `statement`. Scope is evaluated against baseline and resulting paths: add checks the new path; remove checks the old path; rename checks both. A patch intersecting a mandatory live constraint MUST parent it. Under standard and deep bootstrap policy, every applicable advisory constraint MUST receive an in-scope review finding.

## 5.2 Story and acceptance

Story and acceptance body schemas: `schemas/claims/story.body.schema.json`, `schemas/claims/acceptance.body.schema.json`. A story is ready only when the active policy’s planning requirements are met and any required readiness gate is valid.

## 5.3 Patch

Operations are **add**, **remove**, **rename**, **edit**, and **replace** only. v1 MUST NOT encode “non-code claim-rendering” operations.

A patch declares `baseline_weave_id`, `baseline_state_root`, `context_witness_id`, and `operations`. File content is stored by blob id. Checkout files are not part of patch identity.

## 5.4 Deterministic file rules

- Paths are NFC, relative, slash-separated, and MUST NOT contain empty, `.`, or `..` components, NUL, or backslash.
- Case-only renames MUST be rejected in v1 under case-insensitive mode.
- Text edits require UTF-8 and `line_ending` `lf` or `crlf`. Binary or mixed-ending files MUST use `replace`.
- A hunk range is a zero-based half-open interval `[start_line, end_line)` over the baseline logical line sequence defined in Appendix D. Intersecting replacements conflict. Insertions at the same point, or at a replacement boundary, conflict. Disjoint hunks apply in descending `start_line`.
- Two adds of the same path, two replacements of the same path, and an edit of a removed path conflict. A rename reserves both old and new paths.

## 5.5 Witness

Witnesses are evidence, not decisions. Kinds: `context`, `test`, `adversarial`, `review`, `integration`. Every witness has a payload (kind `witness`) and an envelope. A correctly hashed but unauthenticated witness MUST NOT satisfy policy.

Integration witnesses bind to a **Proposal id**, never to a WeaveProposal id. Reviews name a **patch or Proposal**, never a WeaveProposal.

# 6 Materialization and conflict

`skein checkout <weave>` writes the files of that weave’s state root. `skein checkout --thread <T>` overlays eligible unlanded patches. `skein sync` compares a working directory to its declared checkout baseline and asserts a patch. Editors write only disposable checkouts.

Conflict detection is mechanical. Structural compatibility MUST NOT by itself admit a merge. Compatible composition MUST still satisfy constraints and pass integration against the composed state.

# 7 Authorship and policy

Every claim records accountable human, drafting role, and model (empty if human-authored). Policy-relevant objects MUST have an envelope. An agent MUST NOT establish authority merely by writing `accountable_human`. Key, policy-update, retract, and gate claims require the human authorization stated by policy.

Default (bootstrap) policy requires review-role separation and, where models are used, reviewer model-family diversity. For deep threads, the land gate signer differs from the accountable human (`second_human_land_signer`).

## 7.1 Policy packs

Schema: `schemas/policy.schema.json`.

**Bootstrap policy**

- File: `schemas/policy/bootstrap.v1.json`
- `policy_id`: `skein.bootstrap.v1`
- Kind-`policy` object id (SHA-256 domain-separated): `4ccde27acc309e97f3d3eb326a246166f3658da2e3b6ccc027c8eb3526b1a066`

Phase 1 MUST use this pack as the only valid local policy. `gate_reuse` is `refuse`. Implementations MUST refuse gate reuse even if a future pack field is added.

A normal WeaveProposal pins `validation_policy_hash`. A **policy-transition proposal** is a Proposal that also contains `resulting_policy_hash`. Phase 1 MUST parse that field and MUST NOT land a policy transition.

Integration plan hash is `blob_id(canonical(policy.integration_plan))`. For the bootstrap pack that value is `c822299d688f3374c5d65f7ac0029e197452d61e1426fde43537ef533dbbf294`.

| Scale | Bootstrap requirements before land |
| --- | --- |
| trivial | patch, test witness, passing integration witness, land gate |
| standard | trivial plus story, acceptance, verification witnesses, independent review |
| deep | standard plus adversarial witness, brief or requirement, readiness gate, second human land signer |

## 7.2 Verification strategies

Policy selects verification evidence. Strategy classes include generic command-result, red-green, reproduction-resolution, before-after, invariant, static analysis, benchmark, and human inspection. Red-green is not required for every standard thread.

# 8 Landing

Landing is a transaction. It admits a closed set of claims to a target weave or changes nothing. Lands against a weave are serialized by its land queue.

## 8.1 Proposal and WeaveProposal

A Proposal contains repository id; target weave id and state root; proposal claim ids; policy id and validation-policy hash; constraint ids; integration-plan hash; composed state root; optional `resulting_policy_hash`.

A WeaveProposal contains Proposal id, required review ids, required verification-witness ids, integration-witness id, and the policy hash that MUST equal the Proposal’s `validation_policy_hash`.

The packet shown to a human is a **non-hashed rendering**. The gate signs the WeaveProposal **object id**:

```
weave_proposal_id = SHA-256( "skein.weave-proposal.v1\0" || canonical(weave_proposal) )
```

For `development-key`, the Ed25519 signature is over `skein.gate.v1\0 || weave_proposal_id_raw_32_bytes`. For `webauthn`, the WebAuthn challenge MUST be those same 32 raw bytes (base64url-encoded as the challenge). Phase 1 MUST parse WebAuthn gate bodies and MUST NOT accept them for land.

## 8.2 Candidate validity

A land MUST occur only when all of the following hold:

1. Every dependency thread has an eligible land in the target weave.
2. All policy-required claims, parents, witnesses, and reviews are present and valid under the Proposal’s pinned validation policy.
3. Every live applicable constraint is parented and every advisory constraint has a current finding.
4. Proposed operations are conflict-free against the target state and within the Proposal.
5. The integration witness names the exact Proposal id, target state root, composed state root, and integration-plan hash, and it passes (`exit_status` 0).
6. A valid human gate of an algorithm accepted by the active profile signs the exact WeaveProposal id using a credential authorized by policy.

There is no second validity list. The gate does not sign a generic “candidate hash.”

## 8.3 Staleness and gate reuse

By default, any target weave advance, policy change, proposal change, applicable-constraint change, review change, integration-plan change, integration-witness change, or failed rerun expires the affected Proposal and any WeaveProposal built from it. The queue MUST build a new Proposal, rerun integration, construct a new WeaveProposal, and require a new gate.

v1 MUST refuse gate reuse. Object fields for a future equivalence rule MAY exist; Phase 1 and bootstrap policy MUST treat `gate_reuse_requested` as a policy failure.

## 8.4 Integration witness

Schema: `schemas/witnesses/integration.schema.json`. Network access is disabled by default. Secrets are unavailable unless a policy declares injection (bootstrap does not). Retry and flaky handling belong to policy; bootstrap records a single command `true` as a local stub plan.

# 9 Agent protocol

Supported verbs: open-thread, assert, project, witness, sync, propose-land, gate, land, port, query, checkout. Request and response JSON and error codes: `abi/phase1.md`.

An implementation agent is offered `project` as its default read. It MUST NOT scan the object store unless granted reach permits that operation.

## 9.1 Projection reach

| Reach | Capability |
| --- | --- |
| projection-only | Projection bundle and permitted blobs only. Phase 1 MUST parse the context witness and MUST NOT claim a production sandbox. |
| checkout-readable | Read a checkout at a fixed weave. |
| tools | Declared tool allowlist; each invocation attached to a witness. |

## 9.2 Review

A review names subject kind `patch` or `proposal`, reviewed state root, policy hash, and findings. A later patch or Proposal change makes a review stale. A review MUST NOT name a WeaveProposal.

## 9.3 Why query

`why(path, weave)` identifies the last land in weave ancestry whose patch operations mention the path, then returns that land id and its semantic parent ids. Line-range attribution is optional and not required for Phase 1. Indexes MUST be rebuildable.

# 10 Credentials and gates

A `gate` claim contains `name`, `algorithm`, `weave_proposal_id`, `credential_id`, and algorithm-specific assertion fields. The repository accepts a Phase 1 land gate only when `algorithm` is `development-key`, the Ed25519 signature verifies as in §8.1, user verification is implied by possession of the local development key, and the credential is a repository-local development identity. Development keys MUST NOT be valid repository-service or production identities.

WebAuthn fields are specified so object shapes do not change in Phase 2. Phase 1 MUST NOT land them.

Credential registration and revocation use `key` claims. Recovery workflows are out of Phase 1; the `key` object shape is frozen.

# 11 Store and transport

Sync between clones transfers missing content-addressed objects and compares named weave pointers. Packet schema: `schemas/sync-packet.schema.json`. Phase 1 MUST parse the packet and MUST NOT apply remote pointer updates.

There is no rebase. A stale local Proposal is rebuilt against the current target weave. A blob MAY be redacted only through a `redact` claim; Phase 1 MUST store the claim and MUST NOT implement tombstone hiding as a security boundary.

## 11.1 Git export

`skein export git` is deferred to Phase 2. Git is not Skein storage or authority.

# 12 Human surfaces

Informative. Thread board, packet, why, and diff are renderings. Diff is never an alternative authority for admission. Packet contents MUST be derived from the WeaveProposal and its referenced objects so a human is signing the same id the machine verifies.

# 13 Conformance

1. Implement Canonical JSON v1; exclude derived ids from hashed payloads; pass `vectors/`.
2. Refuse mutation of stored claims; refuse malformed objects (`error: malformed`).
3. Reconstruct the same state root by walking parent-linked weaves.
4. Implement v1 path and hunk conflict rules; never admit a merge on structural compatibility alone.
5. Refuse a gate that does not verify over the exact WeaveProposal id against an authorized credential of an algorithm the profile accepts.
6. Refuse land when the pinned policy is unsatisfied, the Proposal or WeaveProposal is stale, or the integration witness does not pass for the composed state.
7. Advance a weave only through serialized land CAS on the prior manifest id.
8. Record a context witness for every `project` call.
9. Answer path-level why from rebuildable graph state.
10. Parse all v1 object schemas; execute only those the active profile trusts.

# 14 Phase 1 behavior profile

Normative subset of **behavior**, not a second object model.

**MUST implement:** local filesystem object store; one named weave `main`; Canonical JSON; authenticated local envelopes; deterministic checkout and patch composition; bootstrap policy evaluation; Proposal and WeaveProposal; `development-key` gate; land CAS; log, diff, why; commands in `abi/phase1.md`.

**MUST parse and store, MUST NOT execute as trustworthy admission:** WebAuthn gates, remote sync packets, redact hiding, policy-transition landing, projection-only sandbox enforcement.

**MUST NOT:** silent rebase; unauthenticated witnesses satisfying policy; automated scale-down; Git as authority; treat development keys as production identities.

# 15 v1 closures of former open questions

| Question | v1 object/behavior freeze |
| --- | --- |
| Canonical JSON and hash agility | Frozen SHA-256 with the prefixes in §4.1. Agility is a new prefix later. |
| Declared-test-input / gate reuse | Fields MAY exist; v1 MUST refuse reuse. |
| Projection-only sandbox substrate | Context witness records reach; enforcement is out of Phase 1. |
| Recovery threshold | `key` claims are specified; recovery protocol is Phase 2. |
| Existing-repository import | Out of Phase 1. `skein init` creates an explicit empty tree and bootstrap policy. |
| Language adapters | Non-normative (Appendix A). |

# 16 Summary

Skein versions decisions and derives code state from accepted decisions. Threads organize work without being branches. Weaves are parent-linked manifests. Patches are deterministic file operations. A Proposal binds a change to an exact target; a WeaveProposal is the exact object a human signs. The core stays language-agnostic.

# Appendix A Extensions (informative)

A language adapter is an out-of-process plugin. An adapter id and version MUST be pinned wherever it affects a resulting state. Conformance never requires one.

# Appendix B Normative test vectors

Executable fixtures live in `vectors/` as `in.json` / `expected.json` pairs. A conforming implementation MUST pass `python tools/vector_runner.py`. Suites:

- `encoding` — Canonical JSON, integers, timestamps, NFC
- `ids` — every hashed kind, derived-id exclusion, blob ids
- `trees` — empty root and sorted entries
- `paths` — normalization and skein.glob.v1
- `hunks` — conflicts, apply order, rename/remove, case-only rename
- `weaves` — parent-linked replay
- `proposals` — Proposal and WeaveProposal ids
- `staleness` — land CAS expire vs land
- `policy` — bootstrap evaluation, scale, thread state
- `policy-transition` — object validity (landing still Phase 2)
- `gates` — development-key accept/reject; WebAuthn not a Phase 1 land
- `why` — last land that touched a path

The vector runner is not a second full VCS. Matching these fixtures is the spec-writing exit criterion. Two complete CLI implementations remain a later proof (Appendix C.6).

# Appendix C Initial implementation (informative)

Unchanged in intent from Draft 0.6 Appendix C: local repository, simulated gate, regular files only, bootstrap policy, Git not used as authority. Command surface is now normatively defined in `abi/phase1.md`. Layout:

```
.skein/objects/          Content-addressed objects and blobs
.skein/refs/weaves/main  Named ref JSON (schemas/named-ref.schema.json)
.skein/repository.json   Repository descriptor
.skein/policy.json       Bootstrap policy reference (id + hash)
.skein/keys/             Development-only local gate and runner keys
.skein/index/            Rebuildable indexes
```

# Appendix D Algorithms (normative)

Implementations MUST match these algorithms. `tools/skein.py` is a convenience encoding of the same rules; vectors are the byte-level judge.

## D.1 canonicalize(value) → bytes

1. If value is null, return `null`. If boolean, return `true` or `false`.
2. If integer, reject if outside −9007199254740991…9007199254740991; else return the decimal ASCII form with no leading zeros (`0` and `-n` as usual).
3. If float, reject.
4. If string, NFC-normalize, then JSON-quote using the escape table in `schemas/canonical-json.md`.
5. If array, return `[` + canonicalize(elements joined by `,`) + `]`.
6. If object, NFC each key; reject duplicate NFC keys; sort keys by code point; return `{` + `canonical_key:canonical_value` pairs joined by `,` + `}`.

## D.2 object_id(kind, payload) and tree_root(entries)

Reject if payload contains `id`. Return lowercase hex of SHA-256(prefix[kind] || canonicalize(payload)).

`tree_root`: normalize each path; require `kind=file`; reject duplicate paths; sort entries by path code point; return `object_id("tree", {entries, schema_version:1})`.

## D.3 normalize_path and ascii_casefold

NFC; reject NUL, backslash, leading/trailing slash, empty, `.`, and `..` components. ASCII case-fold maps A–Z to a–z only.

## D.4 logical_lines and apply_hunks

Separator is `\n` for `lf` and `\r\n` for `crlf`. Empty string yields no lines. Otherwise `text.split(separator)`, which retains a trailing empty line when the file ends in a terminator.

Hunks conflict as in §5.4. Apply remaining hunks in descending `start_line`. Join with the same separator.

## D.5 compose(target, operations)

Detect structural conflicts (two adds, edit of removed path, reserved rename paths, case-only rename in case-insensitive mode, hunk conflicts). If any, fail with `error: conflict`. Otherwise apply operations in order to a path→blob_id map. `edit` UTF-8-decodes the blob, applies hunks, and stores a new blob id.

## D.6 skein.glob.v1

`*` matches within one path segment. `?` matches one non-slash character. `**` matches across segments. A pattern containing no `/` MAY match in any directory. A pattern containing `/` is matched from the repository root.

## D.7 evaluate_policy(policy, scale, evidence)

Look up `policy.scales[scale]`. For each boolean requirement that is true, require the corresponding evidence flag. Always fail if `witness_authenticated` is false, if `automated_scale_down` is true, or if `gate_reuse_requested` is true. Return the list of missing requirement ids; empty means pass.

## D.8 land_cas(current_weave_id, proposal, …)

If `proposal.target_weave_id != current_weave_id`, fail `stale`. If policy, integration, or gate checks fail, fail `policy` or `unauthenticated` as specified. Otherwise write the land claim, write the child weave with `parent=current_weave_id`, and CAS the named ref.

## D.9 derive_thread_state and effective_scale

See §4.4. `effective_scale` ignores invalid assertions; non-human issuers may only increase rank; human issuers may decrease only with `authorized_down`.

## D.10 why(path, lands)

Walk lands from newest to oldest. Return the first whose `patch_paths` contains the normalized path, with that land’s parent id list. If none, return no chain.

# Appendix E Claim type catalog (normative)

See `schemas/claims/catalog.json` for issuer class, thread-field rules, required parent types, and body schema paths.

| type | issuer | thread field | required parent types |
| --- | --- | --- | --- |
| thread | human | forbidden | none |
| constraint | human | optional | none |
| brief | human | required | thread |
| requirement | human | required | thread |
| story | human, agent | required | thread |
| acceptance | human, agent | required | story |
| scale_assertion | human, agent | required | thread |
| patch | human, agent | required | thread |
| review | human, agent | required | none (subject in body) |
| verdict | human | required | thread |
| gate | human | required | none (WeaveProposal id in body) |
| land | runner | required | gate |
| retract | human | optional | land |
| redact | human | optional | none |
| key | human | forbidden | none |
| policy_update | human | forbidden | none |

Required parent types mean at least one parent claim of that type MUST appear in `parents` (the `thread` parent for thread-scoped claims is the thread opener id, which also appears in `thread`).
