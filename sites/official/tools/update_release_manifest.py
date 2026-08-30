#!/usr/bin/env python3
"""Validate and append one immutable APK release to the public manifest."""

from __future__ import annotations

import argparse
import datetime as dt
import hashlib
import json
import pathlib
import re
import sys


HEX_64 = re.compile(r"^[0-9a-f]{64}$")
VERSION = re.compile(r"^[0-9]+\.[0-9]+\.[0-9]+(?:-[0-9A-Za-z.-]+)?$")
COMMIT = re.compile(r"^[0-9a-f]{40}$")


def fail(message: str) -> None:
    raise ValueError(message)


def parse_timestamp(value: str) -> str:
    if not value.endswith("Z"):
        fail("publishedAt must be UTC and end in Z")
    try:
        dt.datetime.fromisoformat(value[:-1] + "+00:00")
    except ValueError as problem:
        fail(f"publishedAt is not RFC3339: {problem}")
    return value


def validate_release(item: object) -> dict:
    if not isinstance(item, dict):
        fail("every release must be an object")
    required = {
        "versionName", "versionCode", "publishedAt", "fileName", "size",
        "sha256", "certificateSha256", "commit", "url",
    }
    if set(item) != required:
        fail(f"release fields differ from the schema: {sorted(set(item) ^ required)}")
    version_name = item["versionName"]
    if not isinstance(version_name, str) or not VERSION.fullmatch(version_name):
        fail("versionName must look like 1.2.3")
    if not isinstance(item["versionCode"], int) or item["versionCode"] <= 0:
        fail("versionCode must be a positive integer")
    parse_timestamp(item["publishedAt"])
    expected_file = f"simple-vpn-{version_name}.apk"
    if item["fileName"] != expected_file:
        fail(f"fileName must be {expected_file}")
    if not isinstance(item["size"], int) or item["size"] <= 0:
        fail("size must be a positive integer")
    for field in ("sha256", "certificateSha256"):
        if not isinstance(item[field], str) or not HEX_64.fullmatch(item[field]):
            fail(f"{field} must be 64 lowercase hexadecimal characters")
    if not isinstance(item["commit"], str) or not COMMIT.fullmatch(item["commit"]):
        fail("commit must be a full lowercase Git SHA")
    expected_url = f"/download/releases/{version_name}/{expected_file}"
    if item["url"] != expected_url:
        fail(f"url must be {expected_url}")
    return item


def read_manifest(path: pathlib.Path) -> dict:
    if not path.exists() or path.stat().st_size == 0:
        return {"schema": 1, "latest": None, "releases": []}
    loaded = json.loads(path.read_text(encoding="utf-8"))
    if not isinstance(loaded, dict) or set(loaded) != {"schema", "latest", "releases"}:
        fail("manifest fields differ from the schema")
    if loaded["schema"] != 1 or not isinstance(loaded["releases"], list):
        fail("unsupported manifest schema")
    releases = [validate_release(item) for item in loaded["releases"]]
    names = [item["versionName"] for item in releases]
    codes = [item["versionCode"] for item in releases]
    if len(names) != len(set(names)) or len(codes) != len(set(codes)):
        fail("manifest contains a duplicate version")
    if releases:
        highest = max(releases, key=lambda item: item["versionCode"])
        if loaded["latest"] != highest["versionName"]:
            fail("latest does not name the highest versionCode")
    elif loaded["latest"] is not None:
        fail("empty manifest must have latest=null")
    return loaded


def main() -> int:
    parser = argparse.ArgumentParser()
    parser.add_argument("--manifest", type=pathlib.Path, required=True)
    parser.add_argument("--output", type=pathlib.Path, required=True)
    parser.add_argument("--version-name", required=True)
    parser.add_argument("--version-code", type=int, required=True)
    parser.add_argument("--published-at", required=True)
    parser.add_argument("--file", type=pathlib.Path, required=True)
    parser.add_argument("--sha256", required=True)
    parser.add_argument("--certificate-sha256", required=True)
    parser.add_argument("--commit", required=True)
    args = parser.parse_args()

    manifest = read_manifest(args.manifest)
    releases = manifest["releases"]
    if any(item["versionName"] == args.version_name for item in releases):
        fail(f"versionName {args.version_name} is already published")
    if any(item["versionCode"] == args.version_code for item in releases):
        fail(f"versionCode {args.version_code} is already published")
    if releases:
        latest = max(releases, key=lambda item: item["versionCode"])
        expected = latest["versionCode"] + 1
        if args.version_code != expected:
            fail(f"versionCode must be the next value {expected}")
        if args.certificate_sha256 != latest["certificateSha256"]:
            fail("signing certificate differs from the established release key")

    if not args.file.is_file() or args.file.stat().st_size <= 0:
        fail("APK file does not exist or is empty")
    if args.file.name != f"simple-vpn-{args.version_name}.apk":
        fail("APK filename does not match versionName")
    actual_sha256 = hashlib.sha256(args.file.read_bytes()).hexdigest()
    if args.sha256 != actual_sha256:
        fail("sha256 does not match the APK bytes")

    entry = validate_release({
        "versionName": args.version_name,
        "versionCode": args.version_code,
        "publishedAt": args.published_at,
        "fileName": args.file.name,
        "size": args.file.stat().st_size,
        "sha256": args.sha256,
        "certificateSha256": args.certificate_sha256,
        "commit": args.commit,
        "url": f"/download/releases/{args.version_name}/{args.file.name}",
    })
    updated = releases + [entry]
    updated.sort(key=lambda item: item["versionCode"], reverse=True)
    result = {"schema": 1, "latest": args.version_name, "releases": updated}
    args.output.write_text(json.dumps(result, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    return 0


if __name__ == "__main__":
    try:
        sys.exit(main())
    except (OSError, ValueError, json.JSONDecodeError) as problem:
        print(f"manifest rejected: {problem}", file=sys.stderr)
        sys.exit(1)
