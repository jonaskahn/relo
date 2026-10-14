#!/usr/bin/env python3
"""Render a flat coverage badge from a Go coverprofile.

Usage: coverage-badge.py <coverage.out> <coverage.svg>

The percentage is the share of instrumented statements the profile marks
covered. The badge is a dependency-free SVG in the flat shields style, so CI
can publish it to the site without another service.
"""

import sys


def total_coverage(path):
    # The gate concatenates one profile per test package, so the same block
    # appears once per package that instruments it. Merge duplicates the way
    # go tool cover does: a statement counts once, covered when any run
    # covered it.
    blocks = {}
    with open(path) as profile:
        for line in profile:
            parts = line.split()
            if len(parts) != 3 or parts[0] == "mode:":
                continue
            statements = int(parts[1])
            covered = int(parts[2]) > 0
            if parts[0] in blocks:
                blocks[parts[0]][1] = blocks[parts[0]][1] or covered
            else:
                blocks[parts[0]] = [statements, covered]
    statements = sum(count for count, _ in blocks.values())
    if statements == 0:
        raise ValueError(f"no statements in {path}")
    covered = sum(count for count, hit in blocks.values() if hit)
    return 100.0 * covered / statements


def color(pct):
    if pct >= 90:
        return "#4c1"
    if pct >= 80:
        return "#97ca00"
    if pct >= 70:
        return "#a4a61d"
    if pct >= 60:
        return "#dfb317"
    if pct >= 50:
        return "#fe7d37"
    return "#e05d44"


def badge(label, value, fill):
    char = 6.4
    left = round(len(label) * char) + 12
    right = round(len(value) * char) + 12
    width = left + right
    return f"""<svg xmlns="http://www.w3.org/2000/svg" width="{width}" height="20" role="img" aria-label="{label}: {value}">
  <rect width="{left}" height="20" fill="#555"/>
  <rect x="{left}" width="{right}" height="20" fill="{fill}"/>
  <g fill="#fff" text-anchor="middle" font-family="Verdana,Geneva,DejaVu Sans,sans-serif" font-size="11">
    <text x="{left / 2}" y="14">{label}</text>
    <text x="{left + right / 2}" y="14">{value}</text>
  </g>
</svg>
"""


def main():
    if len(sys.argv) != 3:
        raise SystemExit("usage: coverage-badge.py <coverage.out> <coverage.svg>")
    pct = total_coverage(sys.argv[1])
    text = f"{pct:.1f}".rstrip("0").rstrip(".") + "%"
    with open(sys.argv[2], "w") as out:
        out.write(badge("coverage", text, color(pct)) + "\n")
    print(text)


if __name__ == "__main__":
    main()
