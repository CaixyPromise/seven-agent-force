#!/usr/bin/env python3
"""Protected Result Processor bootstrap.

This program is part of the admitted Worker image rather than a user supplied
Processor archive.  It deliberately keeps the Processor contract off stdout:
the only authoritative candidate is the atomically written JSON file selected
by ``--result``.  Stdout/stderr remain bounded diagnostics owned by the
Supervisor.

The bootstrap accepts only a release root, a bounded context file and a result
path under the current Attempt state directory.  It neither installs packages
nor imports from the repository/current directory/user site-packages.
"""

from __future__ import annotations

import argparse
import dataclasses
import importlib.util
import json
import os
import pathlib
import subprocess
import sys
import tempfile
from typing import Any


MAX_CONTEXT_BYTES = 1 << 20
MAX_RESULT_BYTES = 1 << 20
MAX_COMMAND_OUTPUT = 256 << 10
MAX_COMMAND_TIMEOUT = 300


class BootstrapError(RuntimeError):
    pass


def _bounded_json(path: pathlib.Path, maximum: int) -> Any:
    body = path.read_bytes()
    if not body or len(body) > maximum:
        raise BootstrapError("bounded JSON input is empty or oversized")
    try:
        return json.loads(body.decode("utf-8"))
    except (UnicodeDecodeError, json.JSONDecodeError) as exc:
        raise BootstrapError("bounded JSON input is invalid") from exc


def _inside(root: pathlib.Path, value: pathlib.Path) -> pathlib.Path:
    root = root.resolve()
    value = value.resolve()
    try:
        value.relative_to(root)
    except ValueError as exc:
        raise BootstrapError("path escapes approved root") from exc
    return value


def _safe_environment(values: dict[str, Any]) -> dict[str, str]:
    env: dict[str, str] = {}
    for key, value in values.items():
        if not isinstance(key, str) or not key or len(key) > 128:
            raise BootstrapError("processor environment key is invalid")
        if not key.replace("_", "a").isalnum() or key[0].isdigit():
            raise BootstrapError("processor environment key is invalid")
        if isinstance(value, (dict, list)):
            env[key] = json.dumps(value, separators=(",", ":"), sort_keys=True)
        elif value is None:
            env[key] = ""
        else:
            env[key] = str(value)
        if len(env[key].encode("utf-8")) > 64 << 10:
            raise BootstrapError("processor environment value is oversized")
    return env


@dataclasses.dataclass(frozen=True)
class CommandResult:
    argv: list[str]
    returncode: int
    stdout: str
    stderr: str
    timed_out: bool = False


class ProcessRunner:
    def __init__(self, repo: pathlib.Path, environment: dict[str, str]):
        self._repo = repo
        self._environment = environment

    def run(
        self,
        argv: list[str],
        *,
        cwd: str | None = None,
        timeout_seconds: int = 120,
    ) -> CommandResult:
        if not isinstance(argv, list) or not argv or len(argv) > 64:
            raise BootstrapError("context.process.run requires a non-empty argv list")
        command: list[str] = []
        for value in argv:
            if not isinstance(value, str) or not value or len(value) > 4096 or "\x00" in value:
                raise BootstrapError("context.process.run argv is invalid")
            command.append(value)
        if not isinstance(timeout_seconds, int) or timeout_seconds < 1 or timeout_seconds > MAX_COMMAND_TIMEOUT:
            raise BootstrapError("context.process.run timeout is invalid")
        location = self._repo if cwd is None else _inside(self._repo, self._repo / cwd)
        command_env = {"PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", "LANG": "C.UTF-8", "LC_ALL": "C.UTF-8"}
        command_env.update(self._environment)
        try:
            completed = subprocess.run(
                command,
                cwd=str(location),
                env=command_env,
                stdin=subprocess.DEVNULL,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=False,
                timeout=timeout_seconds,
                check=False,
                start_new_session=True,
            )
        except subprocess.TimeoutExpired as exc:
            return CommandResult(
                argv=command,
                returncode=-1,
                stdout=_decode_bounded(exc.stdout),
                stderr=_decode_bounded(exc.stderr),
                timed_out=True,
            )
        return CommandResult(
            argv=command,
            returncode=completed.returncode,
            stdout=_decode_bounded(completed.stdout),
            stderr=_decode_bounded(completed.stderr),
        )


def _decode_bounded(value: bytes | None) -> str:
    value = value or b""
    if len(value) > MAX_COMMAND_OUTPUT:
        value = value[:MAX_COMMAND_OUTPUT]
    return value.decode("utf-8", errors="replace")


class Workspace:
    def __init__(self, repo: pathlib.Path):
        self.repo = str(repo)


class ProcessorContext:
    def __init__(self, body: dict[str, Any], repo: pathlib.Path, environment: dict[str, str]):
        self.task_id = _required_string(body, "taskId")
        self.attempt_id = _required_string(body, "attemptId")
        self.cycle_id = _required_string(body, "cycleId")
        self.cycle_number = _required_positive_int(body, "cycleNumber")
        self.parameters = body.get("parameters", {})
        if not isinstance(self.parameters, dict):
            raise BootstrapError("parameters must be an object")
        self.workspace = Workspace(repo)
        self.environment = dict(environment)
        self.process = ProcessRunner(repo, environment)


def _required_string(body: dict[str, Any], key: str) -> str:
    value = body.get(key)
    if not isinstance(value, str) or not value or len(value) > 256:
        raise BootstrapError(f"{key} is invalid")
    return value


def _required_positive_int(body: dict[str, Any], key: str) -> int:
    value = body.get(key)
    if not isinstance(value, int) or value < 1:
        raise BootstrapError(f"{key} is invalid")
    return value


def _configure_import_path(release_root: pathlib.Path) -> None:
    runtime_root = pathlib.Path("/opt/seven-worker/result-runtime").resolve()
    # Preserve only interpreter stdlib directories.  In particular, do not
    # inherit the source checkout, cwd, PYTHONPATH or user site-packages.
    preserved = []
    for entry in sys.path:
        if not entry:
            continue
        resolved = pathlib.Path(entry).resolve()
        text = str(resolved)
        if "site-packages" in text or "dist-packages" in text:
            continue
        if resolved == release_root or resolved == runtime_root:
            continue
        preserved.append(text)
    sys.path[:] = [str(release_root), str(runtime_root), *preserved]
    os.environ.pop("PYTHONPATH", None)
    os.environ.pop("PYTHONHOME", None)


def _load_entry(release_root: pathlib.Path, entry_name: str):
    entrypoint = _inside(release_root, release_root / "processor.py")
    if not entrypoint.is_file():
        raise BootstrapError("processor.py is missing")
    spec = importlib.util.spec_from_file_location("__seven_result_processor__", entrypoint)
    if spec is None or spec.loader is None:
        raise BootstrapError("processor entrypoint cannot be loaded")
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    entry = getattr(module, entry_name, None)
    if not callable(entry):
        raise BootstrapError(f"processor.py must expose {entry_name}(context)")
    return entry


def _contract_object(value: Any) -> dict[str, Any]:
    if dataclasses.is_dataclass(value):
        value = dataclasses.asdict(value)
    elif hasattr(value, "to_dict") and callable(value.to_dict):
        value = value.to_dict()
    if not isinstance(value, dict):
        raise BootstrapError("process(context) must return a Result Contract object")
    body = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")
    if not body or len(body) > MAX_RESULT_BYTES:
        raise BootstrapError("Result Contract is empty or oversized")
    return json.loads(body.decode("utf-8"))


def _delivery_object(value: Any) -> dict[str, Any]:
    """Normalize an optional deliver(context) Fixture response.

    A Fixture is allowed to prove that the optional entrypoint is importable and
    returns a bounded declarative action envelope. It has no network or secret
    materialization and this bootstrap never executes the returned action.
    """
    if dataclasses.is_dataclass(value):
        value = dataclasses.asdict(value)
    elif hasattr(value, "to_dict") and callable(value.to_dict):
        value = value.to_dict()
    if not isinstance(value, dict) or set(value) != {"actions"} or not isinstance(value["actions"], list):
        raise BootstrapError("deliver(context) must return an actions object")
    body = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")
    if not body or len(body) > MAX_RESULT_BYTES:
        raise BootstrapError("deliver(context) actions are empty or oversized")
    return json.loads(body.decode("utf-8"))


def _write_atomic(target: pathlib.Path, value: dict[str, Any]) -> None:
    parent = target.parent
    parent.mkdir(parents=True, exist_ok=True)
    body = json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode("utf-8")
    if len(body) > MAX_RESULT_BYTES:
        raise BootstrapError("Result Contract is oversized")
    descriptor, temporary = tempfile.mkstemp(prefix=".result-", dir=parent)
    try:
        with os.fdopen(descriptor, "wb") as handle:
            handle.write(body)
            handle.flush()
            os.fsync(handle.fileno())
        os.chmod(temporary, 0o600)
        os.replace(temporary, target)
    finally:
        try:
            os.unlink(temporary)
        except FileNotFoundError:
            pass


def main() -> int:
    parser = argparse.ArgumentParser(add_help=False)
    parser.add_argument("--processor-root", required=True)
    parser.add_argument("--context", required=True)
    parser.add_argument("--result", required=True)
    parser.add_argument("--materialization", default="")
    parser.add_argument("--entry", choices=("process", "deliver"), default="process")
    args = parser.parse_args()
    root = pathlib.Path(args.processor_root)
    context_path = pathlib.Path(args.context)
    result_path = pathlib.Path(args.result)
    if not root.is_dir() or not context_path.is_file():
        raise BootstrapError("processor root or evaluation context is unavailable")
    _configure_import_path(root.resolve())
    body = _bounded_json(context_path, MAX_CONTEXT_BYTES)
    if not isinstance(body, dict):
        raise BootstrapError("evaluation context must be an object")
    environment: dict[str, str] = {}
    materialization = pathlib.Path(args.materialization) if args.materialization else None
    if materialization is not None:
        raw = _bounded_json(materialization, 1 << 20)
        # The Supervisor carries the one-cycle payload as ``variables``.  The
        # older ``environment`` spelling is accepted only so an already
        # materialized private file can safely be replayed after a Worker
        # process restart; neither spelling is part of the Processor's Result
        # Contract or persisted audit data.
        raw_environment = raw.get("variables", raw.get("environment", {})) if isinstance(raw, dict) else None
        if not isinstance(raw, dict) or not isinstance(raw_environment, dict):
            raise BootstrapError("evaluation materialization is invalid")
        environment = _safe_environment(raw_environment)
        # A secret can only be consumed once after the Agent process group has
        # terminated.  Remove its private source before importing user code.
        materialization.unlink(missing_ok=True)
    repo = pathlib.Path(_required_string(body, "workspaceRepo"))
    if not repo.is_dir():
        raise BootstrapError("workspace repository is unavailable")
    # Expose approved material only to this protected bootstrap process and
    # its direct Processor child commands. It never appears in Docker env,
    # argv, Attempt spec, logs, Result Contract or an Artifact.  Restore the
    # inherited process environment immediately afterwards so a later control
    # action cannot accidentally reuse a prior cycle's secret.
    previous = {key: os.environ.get(key) for key in environment}
    os.environ.update(environment)
    try:
        entry = _load_entry(root.resolve(), args.entry)
        value = entry(ProcessorContext(body, repo.resolve(), environment))
        contract = _contract_object(value) if args.entry == "process" else _delivery_object(value)
    finally:
        for key, old_value in previous.items():
            if old_value is None:
                os.environ.pop(key, None)
            else:
                os.environ[key] = old_value
    _write_atomic(result_path, contract)
    return 0


if __name__ == "__main__":
    try:
        raise SystemExit(main())
    except BootstrapError as exc:
        print(f"result processor bootstrap: {exc}", file=sys.stderr)
        raise SystemExit(42)
    except Exception as exc:  # pragma: no cover - safe boundary for user code
        print(f"result processor bootstrap: processor crashed: {type(exc).__name__}", file=sys.stderr)
        raise SystemExit(43)
