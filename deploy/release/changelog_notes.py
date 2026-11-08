"""Print one version's section of CHANGELOG.md, for the GitHub release notes."""
from __future__ import annotations

import argparse
from pathlib import Path
import re
import sys

ROOT = Path(__file__).resolve().parents[2]


def section(text: str, version: str) -> str:
    """The lines under "## <version>" (or "## [<version>] - date") up to the next "## " heading."""
    version = version.strip().removeprefix("v")
    head = re.compile(r"^##\s+\[?v?" + re.escape(version) + r"\]?(?:\s|$)")
    lines = text.splitlines()
    for start, line in enumerate(lines):
        if head.match(line):
            body = []
            for line in lines[start + 1:]:
                if line.startswith("## "):
                    break
                body.append(line)
            notes = "\n".join(body).strip()
            if notes:
                return notes + "\n"
            break
    raise ValueError(f"CHANGELOG.md has no notes for {version}")


def main() -> None:
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("version", help="1.2.3 or v1.2.3")
    parser.add_argument("--changelog", default=str(ROOT / "CHANGELOG.md"))
    args = parser.parse_args()
    try:
        sys.stdout.write(section(Path(args.changelog).read_text(encoding="utf-8"), args.version))
    except ValueError as e:
        sys.exit(str(e))


if __name__ == "__main__":
    main()
