# Skein Canonical JSON v1

**Normative.** Byte identity of every hashed structured object SHALL be computed from this profile. Hash algorithm for v1 is SHA-256 only. Agility, if ever required, SHALL introduce a new domain prefix and `schema_version`; it SHALL NOT be an in-band switch inside v1 objects.

This file is the sidecar identity contract. [skein-spec-0.7.docx](../skein-spec-0.7.docx) states the same rules in narrative form. On conflict, this file plus `vectors/encoding` and `vectors/ids` are authoritative for hashed bytes.

## 1. Encoding

A conforming implementation MUST produce exactly these bytes:

1. Encode the in-memory value as UTF-8 JSON with **no insignificant whitespace**.
2. Every string (keys and values) MUST be Unicode NFC before emission. If two keys NFC-normalize to the same string, the object is malformed.
3. Object keys MUST be sorted by Unicode code point of the NFC key (not locale collation, not UTF-16 code units as a primary key). Nested objects are sorted independently the same way.
4. Arrays preserve author order.
5. Numbers MUST be JSON integers in the IEEE-safe range **−9007199254740991 … 9007199254740991** (`-(2^53)+1` … `(2^53)−1`). Leading zeros, `+`, decimal points, and scientific notation are forbidden. The canonical form of zero is `0`.
6. Floating-point values MUST be rejected.
7. Literals are `true`, `false`, and `null` (lowercase). Hashed v1 payloads SHOULD omit null optional fields rather than emit `null`, except where a schema requires the key.
8. Timestamps that appear in hashed payloads MUST be RFC 3339 UTC with millisecond precision and a `Z` suffix: `YYYY-MM-DDTHH:MM:SS.sssZ`.
9. Unknown fields in a hashed v1 payload MUST be rejected (schema validation), not round-tripped.

## 2. JSON string escapes

Inside quoted strings, the producer MUST:

- Escape `"` as `\"` and `\` as `\\`.
- Escape U+0008, U+0009, U+000A, U+000C, U+000D as `\b`, `\t`, `\n`, `\f`, `\r` respectively.
- Escape every other U+0000–U+001F code point as `\u00xx` with lowercase hex.
- Emit all other Unicode (after NFC) as UTF-8. The solidus `/` MUST NOT be escaped.

## 3. Domain-separated object IDs

Let `canonical(payload)` be the bytes from §1. Let `prefix` be the UTF-8 domain string plus a NUL byte. Then:

```
id = lowercase_hex( SHA-256( prefix || canonical(payload) ) )
```

Except blobs:

```
blob_id = lowercase_hex( SHA-256( "skein.blob.v1\0" || raw_bytes ) )
```

The derived `id` MUST NOT appear inside the hashed payload. Creation time, storage location, server receipts, and display timestamps belong on the issuance envelope (kind `envelope`) or in non-hashed working files.

### Prefix table

| Kind | Prefix bytes (UTF-8 plus NUL) |
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

No other hashed kinds exist in v1. Named refs (`main`) are mutable pointers, not hashed objects.

## 4. Canonical tree and empty root

A tree payload is:

```json
{"entries":[{"blob_id":"<64 hex>","kind":"file","path":"<nfc path>"}],"schema_version":1}
```

`entries` MUST be sorted by path code point. v1 `kind` MUST be `file`. Duplicate paths are malformed.

```
state_root = object_id("tree", tree_payload)
```

The empty tree is `{"entries":[],"schema_version":1}`. Its root is published as a vector in `vectors/trees/empty` and MUST be reproduced by every implementation.

## 5. Envelope vs payload

An issuance envelope is a separate hashed object of kind `envelope`. It binds `object_kind`, `object_id`, issuer, method, signature, and `issued_at`. Envelope bytes MUST NOT be concatenated into the payload ID of the object they authenticate.

## 6. Test vectors

Conformance requires passing `vectors/encoding` and `vectors/ids`. Implementations MUST NOT use `json.dumps` / `JSON.stringify` as a substitute for this profile.
