#!/usr/bin/env python3
"""Regenerates the file map in DESIGN.md from specs/**/*.md front matter.
Usage: python docs/design/specs/tools/build-index.py (from the repo root)"""
import pathlib
import re
import sys

root = pathlib.Path(__file__).resolve().parents[2]
specs = root / "specs"
index = root / "DESIGN.md"
START, END = "<!-- file-map:start -->", "<!-- file-map:end -->"


def front(p):
    m = re.match(r"---\n(.*?)\n---\n", p.read_text(), re.S)
    fm = {}
    for line in (m.group(1).splitlines() if m else []):
        if ":" in line:
            k, v = line.split(":", 1)
            fm[k.strip()] = v.strip()
    return fm


groups = [("foundations", "Foundations"), ("shell", "Shell"), ("components", "Components"), ("pages", "Pages")]
out = []
for folder, title in groups:
    files = sorted((specs / folder).glob("*.md"))
    if not files:
        continue
    out += [f"### {title}", "", "| File | Covers |", "|---|---|"]
    for f in files:
        fm = front(f)
        rel = "./" + f.relative_to(root).as_posix()
        out.append(f"| `{rel}` | {fm.get('description', '')} |")
    out.append("")

text = index.read_text()
if START not in text or END not in text:
    sys.exit("DESIGN.md is missing the file-map markers")
new = text.split(START)[0] + START + "\n\n" + "\n".join(out).rstrip() + "\n\n" + END + text.split(END)[1]
index.write_text(new)
print(f"file map updated: {sum(1 for _ in specs.rglob('*.md'))} spec files")
