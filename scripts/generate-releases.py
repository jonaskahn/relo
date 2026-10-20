#!/usr/bin/env python3
"""Write the release catalog the site and the console read."""

import argparse
import json
import re
from datetime import datetime, timezone
from pathlib import Path

INSTALL_SCRIPT = "https://relo.ifelse.one/install.sh"
INSTALL_POWERSHELL = "https://relo.ifelse.one/install.ps1"

# One rule per artifact the release workflow publishes. A name no rule
# recognizes keeps its place in the catalog as "other", so a new artifact is
# never silently dropped from the site.
ASSET_RULES = (
    (re.compile(r"^Relo-(?P<version>.+)-universal\.zip$"), "macos", "universal", "app", "zip"),
    (re.compile(r"^Relo-(?P<version>.+)-arm64\.zip$"), "macos", "arm64", "app", "zip"),
    (re.compile(r"^Relo-Setup-(?P<version>.+)-(?P<arch>amd64|arm64)\.exe$"), "windows", None, "installer", "exe"),
    (re.compile(r"^Relo-Setup-(?P<version>.+)-(?P<arch>amd64|arm64)\.deb$"), "linux", None, "installer", "deb"),
    (re.compile(r"^Relo-Setup-(?P<version>.+)-(?P<arch>amd64|arm64)\.rpm$"), "linux", None, "installer", "rpm"),
    (re.compile(r"^relo-(?P<version>.+)-(?P<os>darwin|linux|windows)-(?P<arch>amd64|arm64)\.tar\.gz$"), None, None, "archive", "tar.gz"),
)

ARCHIVE_OS = {"darwin": "macos", "linux": "linux", "windows": "windows"}


def parse_arguments():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("releases_path", type=Path, help="releases array from the GitHub API")
    parser.add_argument("catalog_path", type=Path)
    arguments = parser.parse_args()
    return arguments


def load_releases(path):
    try:
        releases = json.loads(path.read_text(encoding="utf-8"))
    except FileNotFoundError:
        raise SystemExit(f"{path} does not exist")
    except json.JSONDecodeError as failure:
        raise SystemExit(f"{path} is not valid JSON: {failure}")
    if not isinstance(releases, list):
        raise SystemExit(f"{path} must hold an array of releases")
    # gh api --paginate --slurp hands back one array per page.
    if releases and all(isinstance(page, list) for page in releases):
        releases = [release for page in releases for release in page]
    return releases


def published_releases(releases):
    published = [release for release in releases if not release.get("draft")]
    if not published:
        raise SystemExit("no published release is left once drafts are dropped")
    missing_tag = [release for release in published if not str(release.get("tag_name") or "").strip()]
    if missing_tag:
        raise SystemExit("a published release carries no tag")
    published.sort(key=lambda release: (release.get("published_at") or "", release["tag_name"]), reverse=True)
    return published


def version_of(tag):
    return tag[1:] if tag.startswith("v") else tag


def channel_of(release):
    return "beta" if release.get("prerelease") else "stable"


def classify(name):
    if name == "checksums.txt":
        return {"os": "any", "arch": "any", "kind": "checksums", "format": "txt"}
    for pattern, os_name, arch, kind, format_name in ASSET_RULES:
        match = pattern.match(name)
        if not match:
            continue
        groups = match.groupdict()
        return {
            "os": os_name or ARCHIVE_OS[groups["os"]],
            "arch": groups.get("arch") or arch or "any",
            "kind": kind,
            "format": format_name,
        }
    suffix = name.rsplit(".", 1)[-1] if "." in name else ""
    return {"os": "any", "arch": "any", "kind": "other", "format": suffix}


def file_entry(asset):
    classified = classify(asset["name"])
    return {
        "name": asset["name"],
        "url": asset["browser_download_url"],
        "size": asset.get("size", 0),
        **classified,
    }


def pinned_install(version):
    return {
        "script": f"curl -fsSL {INSTALL_SCRIPT} | sh -s -- --version {version}",
        "powershell": f"& ([scriptblock]::Create((irm {INSTALL_POWERSHELL}))) -Version {version}",
        "npm": f"npm i -g @jonaskahn/relo@{version}",
    }


def channel_install():
    return {
        "stable": {
            "script": f"curl -fsSL {INSTALL_SCRIPT} | sh",
            "powershell": f"irm {INSTALL_POWERSHELL} | iex",
            "brew": "brew install jonaskahn/apps/relo",
            "npm": "npm i -g @jonaskahn/relo",
        },
        "beta": {
            "script": f"curl -fsSL {INSTALL_SCRIPT} | sh -s -- --beta",
            "powershell": f"& ([scriptblock]::Create((irm {INSTALL_POWERSHELL}))) -Beta",
            "brew": "brew install jonaskahn/apps/relo-beta",
            "npm": "npm i -g @jonaskahn/relo@beta",
        },
    }


def release_entry(release):
    tag = release["tag_name"]
    version = version_of(tag)
    return {
        "tag": tag,
        "version": version,
        "name": release.get("name") or tag,
        "channel": channel_of(release),
        "draft": False,
        "published_at": release.get("published_at") or "",
        "page": release.get("html_url") or "",
        "notes": release.get("body") or "",
        "install": pinned_install(version),
        "files": [file_entry(asset) for asset in release.get("assets", [])],
    }


def newest_tag(entries, channel):
    for entry in entries:
        if entry["channel"] == channel:
            return entry["tag"]
    return None


def build_catalog(releases):
    entries = [release_entry(release) for release in releases]
    latest = newest_tag(entries, "stable")
    if latest is None:
        raise SystemExit("no stable release is published")
    return {
        "generated": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "latest": latest,
        "latest_beta": newest_tag(entries, "beta"),
        "install": channel_install(),
        "releases": entries,
    }


def write_catalog(catalog, path):
    path.parent.mkdir(parents=True, exist_ok=True)
    body = json.dumps(catalog, indent=2, ensure_ascii=False) + "\n"
    path.write_text(body, encoding="utf-8")


def main():
    arguments = parse_arguments()
    catalog = build_catalog(published_releases(load_releases(arguments.releases_path)))
    write_catalog(catalog, arguments.catalog_path)
    print(
        f"wrote {arguments.catalog_path}: {len(catalog['releases'])} releases, "
        f"latest {catalog['latest']}, beta {catalog['latest_beta'] or 'none'}",
    )


if __name__ == "__main__":
    main()
