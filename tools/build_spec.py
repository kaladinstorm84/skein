"""Regenerate skein-spec-0.7.docx from spec/skein-spec-0.7.md via pandoc."""

from __future__ import annotations

import subprocess
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SRC = ROOT / "spec" / "skein-spec-0.7.md"
DST = ROOT / "skein-spec-0.7.docx"


def main() -> int:
    if not SRC.is_file():
        print(f"missing {SRC}", file=sys.stderr)
        return 1
    cmd = [
        "pandoc",
        str(SRC),
        "-o",
        str(DST),
        "--from",
        "markdown",
        "--to",
        "docx",
        "--verbose",
        "-s",
        "--metadata",
        "title=Skein Spec 0.7",
        "--metadata",
        "subtitle=A version control system for agentic development",
    ]
    print("running", " ".join(cmd), flush=True)
    proc = subprocess.run(cmd, cwd=ROOT)
    if proc.returncode != 0:
        return proc.returncode
    print(f"wrote {DST} ({DST.stat().st_size} bytes)")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
