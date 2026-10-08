#!/usr/bin/env python3
"""Read, check, and bump the product version everywhere it is declared.

A release is only correct when every declaration agrees with the tag, so this
script treats the version as one value with several spellings instead of
letting each file own a copy that drifts.

  version.py current
  version.py check [TAG]
  version.py bump MAJOR|MINOR|PATCH|PRE|VERSION

check with a TAG asserts the files equal that tag as well as each other. bump
rewrites every declaration in place and prints the new value.
"""

import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent

# Each declaration is the file, a regex that finds the declaration, and the
# style used to rewrite it. A file that is absent is skipped, so a tree
# without the npm package still checks everything else.
DECLARATIONS = [
    ("Makefile", r"^VERSION \?= (.+)$", "makefile"),
    ("daemon/internal/cli/root.go", r'^\tversion = "(.+)"$', "go"),
    ("console/package.json", r'^\t"version": "(.+)",$', "json"),
    ('packaging/npm/package.json', r'^\t"version": "(.+)",$', "json"),
    ("console/package-lock.json", None, "json-lock"),
]

SEMVER = re.compile(r"^(\d+)\.(\d+)\.(\d+)(?:-(.+))?$")

# The lockfile keeps the project version twice: in the root object and in
# packages[""]. Both sit above the first dependency entry, so only the text
# before the first dependency name is a declaration.
LOCKFILE_HEAD = re.compile(r'^\t\t\t"version": '"(.+)"',$', re.MULTILINE)


def lockfile_head(text):
    """Returns the text holding only the lockfile project version."""
    first_dependency = re.search(r'^\t\t\t\t"[a-z@]', text, re.MULTILINE)
    return text[: first_dependency.start()] if first_dependency else text


def read(path, pattern):
    """Returns the first capture of pattern in path, or None when absent."""
    if not path.exists():
        return None
    text = path.read_text()
    if pattern is None:
        match = re.search(r'^\t\t\t"version": "(.+)",$', lockfile_head(text), re.MULTILINE)
    else:
        match = re.search(pattern, text, re.MULTILINE)
    return match.group(1) if match else None


def declarations():
    """Yields (relative path, value) for every file that declares a version."""
    for name, pattern, _style in DECLARATIONS:
        value = read(ROOT / name, pattern)
        if value is not None:
            yield name, value


def current():
    """Returns the version every declaration agrees on, or fails loudly."""
    found = list(declarations())
    if not found:
        sys.exit("version: no version declaration found")
    values = {value for _name, value in found}
    if len(values) > 1:
        detail = "\n".join(f"  {name}: {value}" for name, value in found)
        sys.exit(f"version: declarations disagree\n{detail}")
    return found[0][1]


def check(tag=None):
    """Asserts the declarations agree, and equal tag when one is given."""
    found = list(declarations())
    fail = 0

    values = {value for _name, value in found}
    if len(values) > 1:
        print("FAIL: version declarations disagree")
        for name, value in found:
            print(f"  {name}: {value}")
        fail = 1

    if tag:
        expected = tag[1:] if tag.startswith("v") else tag
        for name, value in found:
            if value != expected:
                print(f"FAIL: {name} is {value}, tag {tag} says {expected}")
                fail = 1

    if fail == 0 and found:
        version = found[0][1]
        suffix = f" and matches {tag}" if tag else " and all agree"
        print(f"OK: version {version} in {len(found)} files{suffix}")
    return 1 if fail else 0


def next_version(version, kind):
    """Returns the version kind moves version to."""
    match = SEMVER.match(version)
    if not match:
        sys.exit(f"version: {version} is not a semantic version")
    major, minor, patch = (int(part) for part in match.groups()[:3])
    prerelease = match.group(4)

    if kind == "major":
        return f"{major + 1}.0.0"
    if kind == "minor":
        return f"{major}.{minor + 1}.0"
    if kind == "patch":
        return f"{major}.{minor}.{patch + 1}"
    if kind == "pre":
        # Open the next patch as a prerelease, or advance one already open:
        # 1.2.3 to 1.2.4-rc.1, then 1.2.4-rc.1 to 1.2.4-rc.2.
        base = f"{major}.{minor}.{patch}" if prerelease else f"{major}.{minor}.{patch + 1}"
        number = re.search(r"(\d+)$", prerelease or "")
        return f"{base}-rc.{int(number.group(1)) + 1 if number else 1}"
    if re.match(r"^\d+\.\d+\.\d+(-.+)?$", kind):
        return kind
    sys.exit(f"version: cannot bump {kind}; use major, minor, patch, pre, or a version")


def write(path, pattern, style, version):
    """Replaces the version in one file, keeping its existing formatting."""
    text = path.read_text()

    if style == "json-lock":
        # Rewrite only the project version, never a dependency pin that
        # shares the key.
        head = lockfile_head(text)
        tail = text[len(head) :]
        head = re.sub(r'^\t\t\t"version": ".+",$', f'\t\t\t"version": "{version}",', head, flags=re.MULTILINE)
        # The root object copy is one tab deep.
        head = re.sub(r'^\t"version": ".+",$', f'\t"version": "{version}",', head, flags=re.MULTILINE)
        path.write_text(head + tail)
        return

    replacement = {
        "makefile": f"VERSION ?= {version}",
        "go": f'\tversion = "{version}"',
        "json": f'\t"version": "{version}",',
    }[style]
    path.write_text(re.sub(pattern, replacement, text, count=1, flags=re.MULTILINE))


def bump(kind):
    """Rewrites every declaration to the next version."""
    bump_to = next_version(current(), kind)

    for name, pattern, style in DECLARATIONS:
        path = ROOT / name
        if not path.exists():
            continue
        write(path, pattern, style, bump_to)
    print(bump_to)


def main(argv):
    command = argv[1] if len(argv) > 1 else "current"

    if command == "current":
        print(current())
        return 0
    if command == "check":
        return check(argv[2] if len(argv) > 2 else None)
    if command == "bump":
        if len(argv) < 3:
            sys.exit("version: bump needs major, minor, patch, pre, or a version")
        bump(argv[2])
        return 0
    sys.exit(f"version: unknown command {command}")


if __name__ == "__main__":
    sys.exit(main(sys.argv))
