#!/usr/bin/env python3
"""Download every published Debian release asset into a fresh suite tree."""

import json
import pathlib
import re
import subprocess
import sys


def api_pages(endpoint):
    page = 1
    while True:
        data = json.loads(subprocess.check_output(
            ["gh", "api", f"{endpoint}?per_page=100&page={page}"], text=True))
        if not isinstance(data, list):
            raise ValueError("expected a GitHub API list")
        yield from data
        if len(data) < 100:
            return
        page += 1


def download(repository, output):
    if not re.fullmatch(r"[A-Za-z0-9_.-]+/[A-Za-z0-9_.-]+", repository):
        raise ValueError("invalid repository name")
    output.mkdir(parents=True, exist_ok=False)
    count = 0
    for release in api_pages(f"repos/{repository}/releases"):
        if release["draft"]:
            continue
        suite = "testing" if release["prerelease"] else "stable"
        for asset in api_pages(f"repos/{repository}/releases/{int(release['id'])}/assets"):
            name = asset["name"]
            if not name.endswith(".deb"):
                continue
            if not re.fullmatch(r"[A-Za-z0-9_.+~%-]+\.deb", name):
                raise ValueError("unsafe release asset filename")
            destination = output / suite / name
            destination.parent.mkdir(exist_ok=True)
            # Never silently overwrite an older release's package.
            with destination.open("xb") as package:
                subprocess.run([
                    "gh", "api", "-H", "Accept: application/octet-stream",
                    f"repos/{repository}/releases/assets/{int(asset['id'])}",
                ], stdout=package, check=True)
            if destination.stat().st_size != asset["size"]:
                raise ValueError(f"incomplete download: {name}")
            count += 1
    if count == 0:
        raise ValueError("no published Debian packages found")
    print(f"Downloaded {count} Debian packages")


if __name__ == "__main__":
    if len(sys.argv) != 3:
        sys.exit("usage: download-release-debs.py OWNER/REPO NEW_DIRECTORY")
    download(sys.argv[1], pathlib.Path(sys.argv[2]))
