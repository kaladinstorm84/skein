## Skein 0.7.0 (Phase 1)

First tagged release: **Spec 0.7** plus the local Phase 1 CLI.

History is a graph of claims. Code is what you get after a land is admitted. This build is a local kernel: bootstrap policy, development-key gates, and a JSON CLI/agent ABI.

### CLI

- `init` → `open-thread` → `checkout` → `sync` → `test` → `propose-land` → `approve` → `land`
- Thread titles/slugs and 12-character id prefixes (`skein threads`); full SHA-256 ids remain identity
- Stdout is ABI JSON; exit 0 iff `"ok": true`
- Same objects via `skein agent` on stdin

### Binaries

Stripped, `CGO_ENABLED=0`, `-trimpath -ldflags="-s -w"`.

| File | Platform |
| --- | --- |
| `skein-windows-amd64.exe` | Windows x64 |
| `skein-linux-amd64` | Linux x64 |
| `skein-linux-arm64` | Linux ARM64 (e.g. Orange Pi) |
| `skein-darwin-amd64` | macOS Intel |
| `skein-darwin-arm64` | macOS Apple silicon |

Checksums: `SHA256SUMS.txt`.

```text
git clone https://github.com/kaladinstorm84/skein.git
# or download a binary above
skein init --path-mode case-sensitive
```

Windows demo: `powershell -NoProfile -File .\demo.ps1`

### Not in this release

WebAuthn production gates, remote sync, redact hiding, policy-transition landing, Git export, and board/packet UI. Later-phase objects are parsed and refused.
