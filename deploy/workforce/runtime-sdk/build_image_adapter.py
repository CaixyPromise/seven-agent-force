#!/usr/bin/env python3
"""Build-only image identity finalizer; never accepts a Task input."""
import argparse
import hashlib
import json
from pathlib import Path
import shlex


def build(root, manifest_path, source_name):
    """Index every copied module/resource; called with fixed image paths in main."""
    source = root / source_name
    if source.parent != root or not source.is_file():
        raise ValueError("fixed image source is required")
    launcher = "#!/bin/sh\nexec /usr/bin/python3 -I " + shlex.quote(str(source)) + "\n"
    (root / "entrypoint").write_text(launcher)
    (root / "entrypoint").chmod(0o555)
    entries = []
    for file in sorted(root.rglob("*")):
        if file.is_file() and file.name != "source-index.json":
            if file.is_symlink():
                raise ValueError("adapter source symlink is forbidden")
            entries.append({"path": str(file.relative_to(root)),
                            "sha256": hashlib.sha256(file.read_bytes()).hexdigest()})
    index = json.dumps(entries, sort_keys=True, separators=(",", ":")).encode()
    (root / "source-index.json").write_bytes(index)
    manifest = json.loads(manifest_path.read_text())
    manifest["adapter"]["binarySha256"] = hashlib.sha256(launcher.encode()).hexdigest()
    manifest["adapter"]["sourceSha256"] = hashlib.sha256(index).hexdigest()
    manifest_path.write_text(json.dumps(manifest, sort_keys=True, separators=(",", ":")))


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--source", required=True)
    args = parser.parse_args()
    build(Path("/opt/seven/adapter"), Path("/opt/seven-worker/contract/manifest.json"), args.source)


if __name__ == "__main__":
    main()
