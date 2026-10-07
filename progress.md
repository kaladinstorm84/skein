# Progress

## 2026-10-07 — GitHub repo identity

README now uses `kaladinstorm84/skein`: Actions badge, clone URL, and CI links.

## 2026-10-07 — GitHub Actions build

Added `.github/workflows/build.yml`: on push/PR/dispatch, Ubuntu runs `go test ./...`, `go vet`, Python `vector_runner.py`, then fails if `generate_vectors.py` drifts from committed `vectors/`. After tests, cross-compiles stripped linux/windows/darwin binaries and uploads them as artifacts. Documented in README. `.gitattributes` pins vector JSON and workflow YAML to LF.

## 2026-10-07 — README UML

Added an object-model class diagram and a Phase 1 land-loop sequence diagram to `README.md` (Mermaid, GitHub-native). Class diagram follows spec kinds: claim subtypes, witness, proposal, weave-proposal, weave, named ref, envelope, derived checkout.

## 2026-10-07 — README terms table

Added a Terms table to `README.md` after the land-loop sketch: spec §2 vocabulary plus checkout, land, and human `thread_ref` / short id. Schema file paths stay in the spec, not the landing table.

## 2026-10-07 — GitHub landing README

Rewrote `README.md` as a GitHub landing page: what Skein is, Phase 1 status table, clone/build/demo first, then spec and layout. Contributor notes (Canonical JSON, conflict rule, vector runner) sit below the product path.

## 2026-10-07 — Human-friendly CLI refs

Content hashes stay 64 hex characters. The CLI now also emits `thread_ref` (title slug) and `*_short` (12-char prefix). `--thread` accepts title or slug; other ids accept a unique prefix. Added `skein threads`. Demo uses `hello-from-demo` and short weave-proposal ids.

## 2026-10-07 — PowerShell end-to-end demo

Added `demo.ps1`: init through land, `why`/`log`, stale reland, and `agent port` refused as `policy`.

```text
powershell -NoProfile -File .\demo.ps1
```

## 2026-10-07 — Phase 1 Go CLI

Implemented `cmd/skein` in Go (stdlib + `golang.org/x/text` for NFC). Custom Canonical JSON; hashed bytes never come from `encoding/json`.

- `go test ./internal/conformance` matches all published `vectors/`
- CLI smoke: init → open-thread → checkout → sync → test → propose-land → approve → land → why; reland is `stale`; `agent port` is `policy`
- Stripped `go build -trimpath -ldflags="-s -w"`: windows/amd64 **3.56 MiB**, linux/arm64 **3.19 MiB**
- Bootstrap argv `true` is a built-in no-op (Windows has no `true` binary)
- Layout: `.skein/objects/<kind>/<id[:2]>/<id>`, exclusive `.skein/lock`

```text
go test ./... -count=1 -v
go build -trimpath -ldflags="-s -w" -o skein.exe ./cmd/skein
```

## 2026-10-07 — Spec 0.7 implementable freeze

Implemented the path from Draft 0.6 to Spec 0.7 (Word narrative + normative sidecars).

**W0** Word 0.7: RFC 2119, sidecar conflict rule, object taxonomy, Gate `algorithm` field, Weave as parent-linked, duplicate §8.2 “candidate hash” list removed, §15 closed for v1 objects.

**W1** Canonical JSON v1 frozen in `schemas/canonical-json.md`. Prefixes for weave, tree, policy, envelope. Empty tree root `cef3d661e4dd7ddc29be947eb6d6a18f897df3c9cddaed4dab683653fdd8a0fb`.

**W2** JSON Schema for hashed and non-hashed v1 objects; claim type catalog under `schemas/claims/`.

**W3** Appendix D algorithms; implemented in `tools/skein.py` and judged by vectors.

**W4** Bootstrap pack `schemas/policy/bootstrap.v1.json`, hash `4ccde27acc309e97f3d3eb326a246166f3658da2e3b6ccc027c8eb3526b1a066`.

**W5** Phase 1 behavior profile in spec §14 (parse later objects; do not trust them).

**W6** `vectors/` (60 fixtures) + `python tools/vector_runner.py` (green).

**W7** `abi/phase1.md` error codes and commands; `README.md` authority table.

### Spec build

```text
python -m pip install -r requirements.txt
python tools/generate_vectors.py
python tools/vector_runner.py
python tools/build_spec.py
```

## 2026-10-07 — Draft 0.6 spec review

Reviewed `skein-spec-0.6.docx` against the two-implementation bar. Verdict: strong design, not yet implementable. That review drove Spec 0.7.
