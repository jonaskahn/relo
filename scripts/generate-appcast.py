#!/usr/bin/env python3
"""Insert one release into a Sparkle appcast."""

import argparse
import os
import re
import tempfile
import urllib.parse
import xml.etree.ElementTree as ET
from pathlib import Path

SPARKLE_NS = "http://www.andymatuschak.org/xml-namespaces/sparkle"
ET.register_namespace("sparkle", SPARKLE_NS)


def sparkle_tag(name):
    return f"{{{SPARKLE_NS}}}{name}"


def parse_arguments():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("appcast_path", type=Path)
    parser.add_argument("version")
    parser.add_argument("build")
    parser.add_argument("archive_url")
    parser.add_argument("archive_length", type=int)
    parser.add_argument("signature")
    parser.add_argument("minimum_system_version")
    parser.add_argument("publication_date")
    arguments = parser.parse_args()

    if arguments.archive_length <= 0:
        parser.error("archive_length must be greater than zero")
    if not re.fullmatch(r"[0-9]+(?:\.[0-9]+)*", arguments.minimum_system_version):
        parser.error("minimum_system_version must contain numeric components")
    parsed_url = urllib.parse.urlparse(arguments.archive_url)
    if parsed_url.scheme != "https" or not parsed_url.netloc:
        parser.error("archive_url must be an absolute HTTPS URL")
    for name in ("version", "build", "signature", "publication_date"):
        if not getattr(arguments, name).strip():
            parser.error(f"{name} must not be empty")
    return arguments


def load_or_create_channel(appcast_path):
    try:
        contents = appcast_path.read_bytes()
    except FileNotFoundError:
        rss = ET.Element("rss", {"version": "2.0"})
        channel = ET.SubElement(rss, "channel")
        ET.SubElement(channel, "title").text = "Relo Updates"
        return ET.ElementTree(rss), channel

    root = ET.fromstring(contents)
    channels = root.findall("channel")
    if len(channels) != 1:
        raise ValueError("Appcast must contain exactly one channel.")
    return ET.ElementTree(root), channels[0]


def ensure_release_is_new(channel, build, archive_url):
    for item in channel.findall("item"):
        existing_build = item.findtext(sparkle_tag("version"))
        enclosure = item.find("enclosure")
        existing_url = enclosure.get("url") if enclosure is not None else None
        if existing_build == build:
            raise ValueError(f"Appcast already contains build {build}.")
        if existing_url == archive_url:
            raise ValueError(f"Appcast already contains enclosure URL {archive_url}.")


def build_item(arguments):
    item = ET.Element("item")
    ET.SubElement(item, "title").text = f"Version {arguments.version}"
    ET.SubElement(item, "pubDate").text = arguments.publication_date
    ET.SubElement(item, sparkle_tag("version")).text = arguments.build
    ET.SubElement(item, sparkle_tag("shortVersionString")).text = arguments.version
    ET.SubElement(item, sparkle_tag("minimumSystemVersion")).text = arguments.minimum_system_version
    ET.SubElement(
        item,
        "enclosure",
        {
            "url": arguments.archive_url,
            "length": str(arguments.archive_length),
            "type": "application/octet-stream",
            sparkle_tag("edSignature"): arguments.signature,
            sparkle_tag("shortVersionString"): arguments.version,
        },
    )
    return item


def write_atomically(tree, appcast_path):
    appcast_path.parent.mkdir(parents=True, exist_ok=True)
    try:
        file_mode = appcast_path.stat().st_mode & 0o777
    except FileNotFoundError:
        file_mode = 0o644
    file_descriptor, temporary_path = tempfile.mkstemp(
        dir=appcast_path.parent,
        prefix=f".{appcast_path.name}.",
        suffix=".tmp",
    )
    try:
        os.fchmod(file_descriptor, file_mode)
        with os.fdopen(file_descriptor, "wb") as temporary_file:
            tree.write(temporary_file, encoding="UTF-8", xml_declaration=True)
            temporary_file.flush()
            os.fsync(temporary_file.fileno())
        os.replace(temporary_path, appcast_path)
    except BaseException:
        try:
            os.unlink(temporary_path)
        except FileNotFoundError:
            pass
        raise


def main():
    arguments = parse_arguments()
    tree, channel = load_or_create_channel(arguments.appcast_path)
    ensure_release_is_new(channel, arguments.build, arguments.archive_url)
    existing_items = channel.findall("item")
    insert_index = list(channel).index(existing_items[0]) if existing_items else len(channel)
    channel.insert(insert_index, build_item(arguments))
    ET.indent(tree, space="  ")
    write_atomically(tree, arguments.appcast_path)


if __name__ == "__main__":
    main()
