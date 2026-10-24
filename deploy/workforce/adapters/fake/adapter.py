"""Image-owned deterministic AgentAdapter. No task-selected scenario or provider IO."""
from __future__ import annotations

import hashlib
import json
import os
from pathlib import Path
import re
import stat
import unicodedata
import uuid
from contextlib import contextmanager

from seven_agent.force.agent import ActivityStatus, AgentAdapter, RoundResult

MARKER = "SEVEN_WORKER_REVIEW_CORRECTION.md"
MARKER_TEXT = "This deterministic marker was written only after the server-authorized review correction contract passed validation.\n"
RESULT_TEXT = "Batch 1 result: restored value.txt to good and produced this result record.\n"
LIMIT = 128 << 10


class FakeFailure(ValueError):
    pass


def require(condition):
    if not condition:
        raise FakeFailure("FAKE_INPUT_INVALID")


def text(value, limit=64 << 10):
    require(isinstance(value, str) and len(value.encode("utf-8")) <= limit and "\0" not in value)
    return value


def identifier(value):
    require(isinstance(value, str) and re.fullmatch(r"[1-9][0-9]{0,18}", value) is not None)
    require(int(value) <= (1 << 63) - 1)
    return value


def path(value):
    value = text(value, 4096)
    p = Path(value)
    require(p.is_absolute() and ".." not in p.parts and not any(ord(c) < 32 for c in value))
    return p


@contextmanager
def directory(p):
    """Walk every component without following links, retaining the directory FD."""
    fd = os.open("/", os.O_RDONLY | os.O_DIRECTORY)
    try:
        for part in p.parts[1:]:
            child = os.open(part, os.O_RDONLY | os.O_DIRECTORY | os.O_NOFOLLOW, dir_fd=fd)
            os.close(fd)
            fd = child
        yield fd
    finally:
        os.close(fd)


def regular(fd, name, missing=False):
    try:
        info = os.stat(name, dir_fd=fd, follow_symlinks=False)
    except FileNotFoundError:
        require(missing)
        return None
    require(stat.S_ISREG(info.st_mode) and info.st_nlink == 1)
    return info


def read(fd, name, *, readonly=False, private=False):
    regular(fd, name)
    opened = os.open(name, os.O_RDONLY | os.O_NOFOLLOW | os.O_NONBLOCK, dir_fd=fd)
    with os.fdopen(opened, "rb") as stream:
        info = os.fstat(stream.fileno())
        require(stat.S_ISREG(info.st_mode) and info.st_nlink == 1 and info.st_size <= LIMIT)
        require(not readonly or info.st_mode & 0o222 == 0)
        require(not private or info.st_mode & 0o077 == 0)
        body = stream.read(LIMIT + 1)
        require(len(body) <= LIMIT)
        return body


def write(fd, name, body):
    """Atomic private replacement; never truncate a repository-controlled inode."""
    require(len(body) <= LIMIT)
    regular(fd, name, missing=True)
    temporary = ".fake-" + uuid.uuid4().hex
    opened = os.open(temporary, os.O_WRONLY | os.O_CREAT | os.O_EXCL | os.O_NOFOLLOW, 0o600, dir_fd=fd)
    try:
        with os.fdopen(opened, "wb") as stream:
            stream.write(body)
            stream.flush()
            os.fsync(stream.fileno())
        os.replace(temporary, name, src_dir_fd=fd, dst_dir_fd=fd)
    finally:
        try:
            os.unlink(temporary, dir_fd=fd)
        except FileNotFoundError:
            pass


def decode(body):
    def unique(pairs):
        result = {}
        for key, value in pairs:
            require(key not in result)
            result[key] = value
        return result
    return json.loads(body.decode("utf-8"), object_pairs_hook=unique)


def completion(inputs):
    workspace = path(inputs["workspace"])
    root = workspace.parent.parent.parent
    require(root.name == inputs["taskId"])
    require(workspace == root / "attempts" / inputs["attemptId"] / "repo")
    source = root / "input" / "attempts" / inputs["attemptId"] / "completion-check.json"
    with directory(source.parent) as fd:
        value = decode(read(fd, source.name, readonly=True))
    require(isinstance(value, dict) and set(value) == {"schema", "completionCheck", "completionCheckDigest", "references"})
    require(value["schema"] == "seven.completion-check-input/v1" and value["references"] == [])
    definition = value["completionCheck"]
    require(isinstance(definition, dict) and not set(definition) - {"mode", "command", "script", "instructions", "referenceAssetIds"})
    require(definition.get("referenceAssetIds", []) == [])
    normalized = {"mode": text(definition.get("mode", ""), 32).strip().upper() or "NONE"}
    for key, limit in (("command", 8192), ("script", 65536), ("instructions", 16384)):
        item = text(definition.get(key, "")).replace("\r\n", "\n").strip()
        require(len(item.encode()) <= limit and all(unicodedata.category(c) != "Cc" or c in "\n\t" for c in item))
        if item:
            normalized[key] = item
    mode = normalized["mode"]
    require(mode in {"NONE", "COMMAND", "SCRIPT"})
    require(("command" in normalized) == (mode == "COMMAND") and ("script" in normalized) == (mode == "SCRIPT"))
    # Match Go encoding/json's struct field order, omitempty and HTML escaping.
    raw = json.dumps(normalized, ensure_ascii=False, separators=(",", ":"))
    for char in "<>&\u2028\u2029":
        raw = raw.replace(char, "\\u%04x" % ord(char))
    digest = hashlib.sha256(raw.encode()).hexdigest()
    require(value["completionCheckDigest"] == digest)
    require(str(source) + " (digest " + digest + ")" in inputs["instruction"])


def validate(inputs, role):
    require(isinstance(inputs, dict) and inputs.get("mode") == role)
    if role == "REVIEW":
        required = {"mode", "workspace", "model", "prompt", "output_schema", "output_path"}
        optional = {"resultCommit", "resultTree", "requirements_path", "review_contract_path"}
        require(required <= set(inputs) and not set(inputs) - required - optional)
        for key in ("resultCommit", "resultTree"):
            if key in inputs:
                text(inputs[key], 128)
        for key in ("requirements_path", "review_contract_path"):
            if key in inputs:
                path(inputs[key])
        text(inputs["prompt"])
        text(inputs["model"], 256)
        path(inputs["output_schema"])
        report = path(inputs["output_path"])
        workspace = path(inputs["workspace"])
        require(bool(report.name) and report.parent != Path("/"))
        require(report.parent != workspace and workspace not in report.parents and report.parent not in workspace.parents)
    else:
        required = {"mode", "taskId", "attemptId", "instruction", "workspace", "output", "state", "temporary", "provider", "feedback"}
        require(required <= set(inputs) and not set(inputs) - required - {"reviewCorrection"})
        identifier(inputs["taskId"])
        identifier(inputs["attemptId"])
        text(inputs["instruction"])
        if inputs.get("feedback") is not None:
            text(inputs["feedback"])
        require(isinstance(inputs["provider"], dict) and set(inputs["provider"]) == {"model"})
        text(inputs["provider"]["model"], 256)
        workspace = path(inputs["workspace"])
        for key in ("state", "output", "temporary"):
            p = path(inputs[key])
            require(p != workspace and workspace not in p.parents and p not in workspace.parents)
        require(len({inputs[k] for k in ("state", "output", "temporary")}) == 3)
        correction = inputs.get("reviewCorrection")
        if correction is not None:
            require(isinstance(correction, dict) and {"sourceReviewRunId", "revisionKind"} <= set(correction))
            require(not set(correction) - {"sourceReviewRunId", "revisionKind", "verdict", "reportPath", "reportDigest", "reasonCode"})
            identifier(correction["sourceReviewRunId"])
            require(correction["revisionKind"] in {"AUTOMATIC", "HUMAN_AUTHORIZED"})
            for value in correction.values():
                text(value, 4096)
    path(inputs["workspace"])


def report(applied):
    result = {"schemaVersion": "review-report/v1", "verdict": "PASS" if applied else "REVISE", "score": 100 if applied else 60,
              "summary": "The server-authorized correction marker is present in the successor ResultCommit." if applied else "Apply the fixed correction through an authorized review correction Turn.",
              "reviewTests": [{"command": "inspect fixed correction marker", "exitCode": 0, "status": "PASSED", "summary": "Inspected the supervisor-prepared repository; no checkout or tests executed by this adapter."}]}
    if not applied:
        result["findings"] = [{"id": "fake-correction-required", "severity": "WARNING", "category": "CORRECTNESS", "title": "Fixed correction marker is missing", "message": "The successor ResultCommit must contain the server-authorized FakeHarness correction marker.", "evidence": {"path": "SEVEN_WORKER_RESULT.md", "line": 1}, "suggestedDirection": "Create the correction Turn from the canonical review report."}]
    return json.dumps(result, separators=(",", ":")).encode()


def batch1_report(repo):
    checks = []
    for name, expected in (("value.txt", b"good\n"), ("batch1-result.txt", RESULT_TEXT.encode())):
        matches = regular(repo, name, missing=True) is not None and read(repo, name) == expected
        checks.append({"command": "inspect fixed " + name, "exitCode": 0 if matches else 1,
                       "status": "PASSED" if matches else "FAILED",
                       "summary": "Fixed fixture contents match." if matches else "Fixed fixture file is missing or contents differ."})
    passed = all(check["exitCode"] == 0 for check in checks)
    return json.dumps({"schemaVersion": "review-report/v1", "verdict": "PASS" if passed else "REVISE",
                       "score": 100 if passed else 60,
                       "summary": "Batch1 fixed result contents match." if passed else "Restore the fixed Batch1 value and result record.",
                       "reviewTests": checks}, separators=(",", ":")).encode()


def build_execution(asset_name, asset_digest):
    """Called only by fixed image entrypoints, never from RunRequest or environment."""
    asset = Path(__file__).absolute().parent / "fixtures" / asset_name
    with directory(asset.parent) as fd:
        body = read(fd, asset.name)
    require(hashlib.sha256(body).hexdigest() == asset_digest)
    fixture = decode(body)
    role, scenario = fixture["role"], fixture["scenario"]
    require((role, scenario) in {("AGENT", "BATCH1_COMPLETION"), ("AGENT", "CORRECTION_CYCLE"), ("REVIEW", "CORRECTION_CYCLE"), ("REVIEW", "BATCH1_RESULT")})

    async def execute(request, context, resume=False):
        inputs = request.inputs
        validate(inputs, role)
        require(not resume or role == "AGENT")
        if scenario == "BATCH1_COMPLETION":
            completion(inputs)
        # This Python sidecar IS the running native fake process.
        await context.started(native_pid=os.getpid())
        await context.emit_activity(key="fake:fixture", title="Run fixed image fixture", status=ActivityStatus.STARTED)
        if role == "REVIEW":
            with directory(path(inputs["workspace"])) as repo:
                if scenario == "BATCH1_RESULT":
                    candidate = batch1_report(repo)
                else:
                    candidate = report(regular(repo, MARKER, missing=True) is not None)
            with directory(path(inputs["output_path"]).parent) as output:
                write(output, path(inputs["output_path"]).name, candidate)
            reference = ""
            summary = "Fake Reviewer wrote a candidate report"
        else:
            with directory(path(inputs["state"])) as state:
                require(os.fstat(state).st_mode & 0o077 == 0)
                existing = regular(state, "fake-session.json", missing=True)
                if resume:
                    require(existing is not None)
                    session = decode(read(state, "fake-session.json", private=True))
                    require(isinstance(session, dict) and set(session) == {"schemaVersion", "kind", "sessionId", "sessionDigest", "round", "taskId", "fixtureDigest"})
                    reference = text(request.session_reference, 512)
                    require(re.fullmatch(r"fake-session-[0-9a-f]{64}", reference) is not None)
                    require(session["schemaVersion"] == 1 and session["kind"] == "fake-session" and session["sessionId"] == reference)
                    require(session["sessionDigest"] == hashlib.sha256(reference.encode()).hexdigest())
                    require(session["taskId"] == inputs["taskId"] and session["fixtureDigest"] == asset_digest)
                    require(type(session["round"]) is int and 1 <= session["round"] < 256)
                    session["round"] += 1
                else:
                    require(existing is None)
                    reference = "fake-session-" + hashlib.sha256(("fake-session\0" + inputs["taskId"] + "\0" + inputs["attemptId"]).encode()).hexdigest()
                    session = dict(schemaVersion=1, kind="fake-session", sessionId=reference, sessionDigest=hashlib.sha256(reference.encode()).hexdigest(), round=1, taskId=inputs["taskId"], fixtureDigest=asset_digest)
                with directory(path(inputs["workspace"])) as repo:
                    files = {}
                    if scenario == "BATCH1_COMPLETION":
                        files["value.txt"] = b"good\n" if resume else b"bad\n"
                        if resume:
                            files["batch1-result.txt"] = RESULT_TEXT.encode()
                    else:
                        content = "# Seven Worker FakeHarness Result\n\nTask: `" + inputs["taskId"] + "`\n\nInstruction received:\n\n> " + inputs["instruction"].replace("\n", "\n> ") + "\n"
                        name = "SEVEN_WORKER_RESULT.md" if resume or inputs.get("reviewCorrection") is not None else "SEVEN_WORKER_DRAFT.md"
                        files[name] = content.encode()
                        if inputs.get("reviewCorrection") is not None:
                            files[MARKER] = MARKER_TEXT.encode()
                    for name, body in files.items():
                        regular(repo, name, missing=True)
                        require(len(body) <= LIMIT)
                    for name, body in files.items():
                        write(repo, name, body)
                write(state, "fake-session.json", json.dumps(session).encode())
            summary = "FakeHarness created deterministic fixture files"
        await context.emit_activity(key="fake:fixture", title="Run fixed image fixture", status=ActivityStatus.SUCCEEDED)
        return RoundResult.finished(summary=summary, session_reference=reference)

    return role, execute


def build_adapter(asset_name, asset_digest):
    role, execute = build_execution(asset_name, asset_digest)
    adapter = AgentAdapter(name="fake", version="batch2.1")

    @adapter.run
    async def run(request, context):
        return await execute(request, context)

    if role == "AGENT":
        @adapter.resume
        async def resume(request, context):
            return await execute(request, context, True)
    return adapter
