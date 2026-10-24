"""Local image-author tools. These commands never contact a platform API."""
import argparse
import asyncio
import hashlib
import importlib.util
import json
from pathlib import Path
import shlex
import sys
from contextlib import nullcontext

from ._project import loaded_project, build_project

from .agent import AgentAdapter
from ._protocol import VERSION, decode, encode


async def simulate_supervisor(adapter, fixture=None):
    """No execution by default. Explicit fixtures opt into a local run."""
    reader = asyncio.StreamReader()
    received = asyncio.Queue(maxsize=64)
    class Writer:
        def write(self, raw):
            received.put_nowait(decode(raw, direction="up"))
        async def drain(self):
            await asyncio.sleep(0)
    session = asyncio.create_task(adapter.serve_streams(reader, Writer()))
    def send(method, key, payload):
        raw = encode(dict(protocol=VERSION, method=method, key=key, payload=payload))
        decode(raw, direction="down")
        reader.feed_data(raw)
    try:
        send("initialize", "dev:init", {})
        ready = await asyncio.wait_for(received.get(), 5)
        if ready["method"] != "ready" or ready.get("replyTo") != "dev:init":
            raise ValueError("handshake failed")
        if fixture is None:
            return {"status": "HANDSHAKE_ONLY", "executions": 0}
        if not isinstance(fixture, dict):
            raise ValueError("fixture inputs must be an object")
        send("execution.start", "dev:start", {"roundKey": "dev:round", "inputs": fixture})
        started = False
        events = 0
        async def result():
            nonlocal started, events
            while True:
                frame = await received.get()
                events += 1
                if frame["method"] == "error":
                    raise ValueError("adapter rejected fixture")
                if frame["method"] == "execution.started":
                    if started: raise ValueError("duplicate native start")
                    started = True
                if frame["method"] == "execution.finished":
                    if not started or frame.get("replyTo") != "dev:start" or frame["payload"].get("outcome") != "FINISHED":
                        raise ValueError("fixture did not finish")
                    return {"status": "LOCAL_FIXTURE_FINISHED", "executions": 1, "events": events}
        return await asyncio.wait_for(result(), 10)
    finally:
        reader.feed_eof()
        await asyncio.wait_for(session, 6)


def load_adapter(path):
    path = Path(path).resolve(strict=True)
    spec = importlib.util.spec_from_file_location("_seven_image_author_adapter", path)
    module = importlib.util.module_from_spec(spec)
    # Module loading executes author code. Only run against trusted local code.
    spec.loader.exec_module(module)
    adapters = [value for value in vars(module).values() if isinstance(value, AgentAdapter)]
    if len(adapters) != 1 or "execution.start" not in adapters[0]._handlers:
        raise ValueError("exactly one AgentAdapter with a run handler is required")
    return adapters[0]


def main(argv=None):
    parser = argparse.ArgumentParser(description="Image-author tools; loading an adapter executes its Python module.")
    sub = parser.add_subparsers(dest="command", required=True)
    for name in ("inspect", "dev", "build"):
        command = sub.add_parser(name)
        command.add_argument("source", type=Path)
        command.add_argument("--module", help="Regular package/module in a pyproject.toml src or flat project")
        if name == "dev":
            command.add_argument("--fixture", type=Path, help="Explicit local RunRequest inputs; default only handshakes, never starts Agent")
        if name == "build":
            command.add_argument("--output", required=True, type=Path)
            command.add_argument("--image-module",
                                 help="Immutable absolute Python source path in the final image")
    check = sub.add_parser("check", help="Validate recorded protocol frames without executing an adapter")
    check.add_argument("transcript", type=Path)
    check.add_argument("--direction", required=True, choices=("up", "down"))
    args = parser.parse_args(argv)
    try:
        if args.command == "check":
            from ._protocol import MAX_FRAME_BYTES, decode
            count = 0
            with args.transcript.open("rb") as stream:
                while raw := stream.readline(MAX_FRAME_BYTES + 1):
                    decode(raw, direction=args.direction)
                    count += 1
            if not count:
                raise ValueError("empty transcript")
            print(json.dumps({"frames": count, "status": "VALID_FRAMES_ONLY"}))
            return 0
        if args.source.is_dir() != bool(args.module):
            raise ValueError("project requires --module; single file does not")
        if args.command == 'build' and bool(args.image_module) == bool(args.module):
            raise ValueError("use --module for projects or --image-module for a file")
        if args.command == 'build' and args.module:
            top = args.module.split('.')[0]
            root, output = args.source.resolve(), args.output.resolve()
            if any(output.is_relative_to(package) for package in (root / top, root / 'src')):
                raise ValueError("build output must be outside source packages")
        with loaded_project(args.source, args.module) if args.module else nullcontext((load_adapter(args.source), None)) as (adapter, files):
            return run_author_command(args, adapter, files)
    except Exception:
        print("RUNTIME_SDK_COMMAND_FAILED", file=sys.stderr)
        return 1


def run_author_command(args, adapter, files):
    manifest = {"id": adapter.name, "version": adapter.version,
                "executable": "/opt/seven/adapter/entrypoint", "args": [],
                "protocol": {"name": "seven.agent-adapter", "min": 1, "max": 1},
                "transport": "stdio", "capabilities": adapter.capabilities}
    if args.command == "inspect":
        print(json.dumps(manifest, sort_keys=True))
    elif args.command == "dev":
        fixture = None
        if args.fixture is not None:
            with args.fixture.open("rb") as stream:
                raw = stream.read((1 << 20) + 1)
            if len(raw)>1<<20: raise ValueError("fixture too large")
            fixture=json.loads(raw)
        print(json.dumps(asyncio.run(simulate_supervisor(adapter,fixture)),sort_keys=True))
    elif args.module:
        build_project(args.output, args.module, files, manifest)
        print("Build assets generated; exact image admission still required.")
    else:
        path = Path(args.image_module)
        if path.parent != Path("/opt/seven/adapter") or path.name in {"entrypoint", "source-index.json", "adapter-manifest.json"}:
            raise ValueError("image module must be an absolute immutable path")
        args.output.mkdir(parents=True, exist_ok=True)
        launcher = "#!/bin/sh\nexec /usr/bin/python3 -I " + shlex.quote(str(path)) + "\n"
        # Exclusive creation avoids overwriting a prior release artifact.
        with (args.output / "entrypoint").open("x") as stream:
            stream.write(launcher)
        (args.output / "entrypoint").chmod(0o555)
        manifest["binarySha256"] = hashlib.sha256(launcher.encode()).hexdigest()
        source_body = args.source.read_bytes()
        with (args.output / path.name).open("xb") as stream:
            stream.write(source_body)
        (args.output / path.name).chmod(0o444)
        entries = sorted([
            {"path": "entrypoint", "sha256": manifest["binarySha256"]},
            {"path": path.name, "sha256": hashlib.sha256(source_body).hexdigest()},
        ], key=lambda entry: entry["path"])
        index = json.dumps(entries, sort_keys=True, separators=(",", ":")).encode()
        with (args.output / "source-index.json").open("xb") as stream:
            stream.write(index)
        (args.output / "source-index.json").chmod(0o444)
        manifest["sourceSha256"] = hashlib.sha256(index).hexdigest()
        with (args.output / "adapter-manifest.json").open("x") as stream:
            json.dump({"adapter": manifest}, stream, sort_keys=True)
        print("Build assets generated; image digest and module index admission still required.")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
