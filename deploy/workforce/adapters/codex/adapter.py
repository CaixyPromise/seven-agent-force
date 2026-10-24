"""Image-owned Codex 0.147.0 App Server Adapter."""
from __future__ import annotations

import json
import os
from pathlib import Path
import re
import stat
import sys

sys.path.insert(0, "/opt/seven-worker/result-runtime")
sys.path.insert(0, str(Path(__file__).resolve().parent))

from seven_agent.force.agent import AgentAdapter
from seven_agent.force.interaction import QuestionRequest
from seven_agent.force.tools import ToolResult
import app_server
from app_server import AppServerFailure, BASE_URL, CODEX_EXECUTABLE, CodexAppServer

SECRET_PLACEHOLDER = "[Assistant message withheld: possible credential content]"
SECRET_PATTERNS = tuple(re.compile(pattern, re.IGNORECASE) for pattern in (
    r"-----BEGIN (?:[A-Z0-9]{1,16} )?(?:ENCRYPTED )?PRIVATE KEY-----",
    r"\bsk-(?:proj-|svcacct-|ant-)?[A-Za-z0-9_-]{12,}",
    r"\b(?:gh[pousr]_|github_pat_)[A-Za-z0-9_]{12,}",
    r"\bxox[baprs]-[A-Za-z0-9-]{12,}", r"\b(?:AKIA|ASIA)[A-Z0-9]{16}\b",
    r"\bBearer[ \t]{1,16}[A-Za-z0-9._~+/-]{8,}=*",
    r"\beyJ[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}\.[A-Za-z0-9_-]{8,}",
    r"\b(?:[A-Z0-9_]{1,48}_)?(?:api[_-]?key|access[_-]?token|refresh[_-]?token|client[_-]?secret|password|passwd|secret|token)[\"']?[ \t]{0,16}[:=][ \t]{0,16}[\"']?[^\s\"',;}{]{4,}",
))
HOOK_METADATA_KEYS = ("SEVEN_TASK_ID", "SEVEN_ATTEMPT_ID", "SEVEN_ATTEMPT_GENERATION",
                      "SEVEN_CONTROL_SOCKET", "SEVEN_HOOK_BUNDLE_ROOT", "SEVEN_HOOK_BUNDLE_DIGEST")


class AdapterFailure(ValueError):
    pass


def safe_assistant_text(value):
    if not isinstance(value, str) or len(value.encode("utf-8", errors="replace")) > 1 << 20 or any(p.search(value) for p in SECRET_PATTERNS):
        return SECRET_PLACEHOLDER
    return value.encode("utf-8", errors="replace")[:16384].decode("utf-8", errors="ignore")


def _text(value, limit, nonempty=True):
    if not isinstance(value, str) or (nonempty and not value.strip()) or "\0" in value or len(value.encode()) > limit:
        raise AdapterFailure("CODEX_INPUT_INVALID")
    return value


def _path(value):
    value = _text(value, 4096)
    path = Path(value)
    if not path.is_absolute() or ".." in path.parts or any(ord(c) < 32 for c in value):
        raise AdapterFailure("CODEX_INPUT_INVALID")
    return path


def _write_review_report(path, value):
    raw = value.encode("utf-8")
    if not raw or len(raw) > 256 << 10:
        raise AdapterFailure("CODEX_REVIEW_REPORT_INVALID")
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL
    if hasattr(os, "O_NOFOLLOW"):
        flags |= os.O_NOFOLLOW
    try:
        descriptor = os.open(path, flags, 0o600)
        with os.fdopen(descriptor, "wb") as stream:
            stream.write(raw)
            stream.flush()
            os.fsync(stream.fileno())
        info = path.lstat()
    except OSError:
        raise AdapterFailure("CODEX_REVIEW_REPORT_INVALID") from None
    if not stat.S_ISREG(info.st_mode) or info.st_size != len(raw):
        raise AdapterFailure("CODEX_REVIEW_REPORT_INVALID")


def hook_metadata(inputs):
    values = {key: os.environ[key] for key in HOOK_METADATA_KEYS if key in os.environ}
    if not values:
        return values
    if not set(HOOK_METADATA_KEYS[:4]) <= values.keys():
        raise AdapterFailure("CODEX_HOOK_METADATA_INVALID")
    for key in HOOK_METADATA_KEYS[:3]:
        value = values[key]
        if not re.fullmatch(r"[1-9][0-9]{0,18}", value) or int(value) > (1 << 63) - 1:
            raise AdapterFailure("CODEX_HOOK_METADATA_INVALID")
    if values["SEVEN_TASK_ID"] != inputs["taskId"] or values["SEVEN_ATTEMPT_ID"] != inputs["attemptId"]:
        raise AdapterFailure("CODEX_HOOK_METADATA_INVALID")
    socket = values["SEVEN_CONTROL_SOCKET"]
    if not socket.startswith("/") or Path(socket).name != "s" or any(part in ("", ".", "..") for part in socket.split("/")[1:]):
        raise AdapterFailure("CODEX_HOOK_METADATA_INVALID")
    root, digest = values.get("SEVEN_HOOK_BUNDLE_ROOT"), values.get("SEVEN_HOOK_BUNDLE_DIGEST")
    if root is not None or digest is not None:
        if digest is None or not re.fullmatch(r"[0-9a-f]{64}", digest) or root != "/opt/seven-worker/managed-hooks/" + digest:
            raise AdapterFailure("CODEX_HOOK_METADATA_INVALID")
    return values


def prepare(inputs, session_reference=""):
    if not isinstance(inputs, dict):
        raise AdapterFailure("CODEX_INPUT_INVALID")
    mode = inputs.get("mode")
    if mode == "AGENT":
        required = {"mode", "taskId", "attemptId", "instruction", "workspace", "output", "state", "temporary", "provider", "feedback", "availableTools"}
        optional = {"reviewCorrection", "interactionQuestionRequest", "interactionResponse", "operationToolResult"}
    elif mode == "REVIEW":
        required = {"mode", "workspace", "model", "prompt", "output_schema", "output_path"}
        optional = {"resultCommit", "resultTree", "requirements_path", "review_contract_path"}
    else:
        raise AdapterFailure("CODEX_INPUT_INVALID")
    if not required <= set(inputs) or set(inputs) - required - optional:
        raise AdapterFailure("CODEX_INPUT_INVALID")
    workspace = _path(inputs["workspace"])
    if mode == "AGENT":
        for key in ("taskId", "attemptId"):
            if not isinstance(inputs[key], str) or not re.fullmatch(r"[1-9][0-9]{0,18}", inputs[key]) or int(inputs[key]) > (1 << 63) - 1:
                raise AdapterFailure("CODEX_INPUT_INVALID")
        for key in ("output", "state", "temporary"):
            _path(inputs[key])
        provider_input = inputs["provider"]
        if not isinstance(provider_input, dict) or set(provider_input) != {"model"}:
            raise AdapterFailure("CODEX_INPUT_INVALID")
        model = _text(provider_input["model"], 256)
        available_tools = inputs["availableTools"]
        if (not isinstance(available_tools, list) or len(available_tools) > len(app_server.PLATFORM_TOOLS)):
            raise AdapterFailure("CODEX_DYNAMIC_TOOL_UNAVAILABLE")
        try:
            frozen_tools = []
            seen = set()
            for available_tool in available_tools:
                if (not isinstance(available_tool, dict)
                        or set(available_tool) != {"name", "description", "inputSchema"}
                        or available_tool["name"] in seen):
                    raise AdapterFailure("CODEX_DYNAMIC_TOOL_UNAVAILABLE")
                frozen_tool = app_server.ToolDefinition(
                    available_tool["name"], available_tool["description"],
                    available_tool["inputSchema"])
                expected = app_server.PLATFORM_TOOLS.get(frozen_tool.name)
                if expected is None or frozen_tool.wire() != expected.wire():
                    raise AdapterFailure("CODEX_DYNAMIC_TOOL_UNAVAILABLE")
                seen.add(frozen_tool.name)
                frozen_tools.append(frozen_tool)
        except Exception:
            raise AdapterFailure("CODEX_DYNAMIC_TOOL_UNAVAILABLE") from None
        prompt = _text(inputs["feedback"], 64 << 10, False) if session_reference else _text(inputs["instruction"], 64 << 10)
        question, response = inputs.get("interactionQuestionRequest"), inputs.get("interactionResponse")
        operation_tool_result = inputs.get("operationToolResult")
        if operation_tool_result is not None:
            if not session_reference or question is not None or response is not None:
                raise AdapterFailure("CODEX_OPERATION_RESTORE_INVALID")
            try:
                operation_tool_result = ToolResult.from_wire(operation_tool_result)
            except Exception:
                raise AdapterFailure("CODEX_OPERATION_RESTORE_INVALID") from None
        if question is not None or response is not None:
            if not isinstance(question, dict) or not isinstance(response, dict) or QuestionRequest.from_wire(question).interaction_key != response.get("interactionKey"):
                raise AdapterFailure("CODEX_INTERACTION_RESTORE_INVALID")
            prompt += "\n\nResume after this previously saved user response. It is context, not authorization:\n"
            prompt += json.dumps({"question": question, "answer": response}, sort_keys=True, ensure_ascii=True)
        state = _path(inputs["state"])
        home, codex_home, provider, env = state / "home", state / "codex", "seven", hook_metadata(inputs)
        env.update(SEVEN_PROVIDER_TOKEN="seven-local-provider-facade", OPENAI_API_KEY="seven-local-provider-facade")
    else:
        operation_tool_result = None
        frozen_tools = ()
        model, prompt = _text(inputs["model"], 256), _text(inputs["prompt"], 64 << 10)
        for key in ("requirements_path", "review_contract_path"):
            if inputs.get(key) is not None:
                _path(inputs[key])
        documents = {key: inputs[key] for key in ("requirements_path", "review_contract_path", "resultCommit", "resultTree") if inputs.get(key) is not None}
        if documents:
            prompt += "\nRead these immutable review inputs before reviewing:\n" + json.dumps(documents, sort_keys=True, ensure_ascii=True)
        home, codex_home, provider, env = Path("/tmp/seven-review-home"), Path("/tmp/seven-review-codex"), "seven-review", {}
    if model != model.strip() or any(ord(c) < 32 or ord(c) == 127 or c in '\\"' for c in model):
        raise AdapterFailure("CODEX_INPUT_INVALID")
    if session_reference and (mode != "AGENT" or not re.fullmatch(r"[A-Za-z0-9_-]{1,512}", session_reference)):
        raise AdapterFailure("CODEX_SESSION_INVALID")
    env.update(HOME=str(home), CODEX_HOME=str(codex_home), PATH="/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin", LANG="C.UTF-8")
    command = [CODEX_EXECUTABLE, "app-server", "--stdio", "--strict-config",
               "-c", f'model_provider="{provider}"', "-c", f'model_providers.{provider}.name="Seven Local Provider Facade"',
               "-c", f'model_providers.{provider}.base_url="{BASE_URL}"', "-c", f'model_providers.{provider}.wire_api="responses"',
               "-c", f'model_providers.{provider}.requires_openai_auth=false']
    if mode == "AGENT":
        # Codex 0.147.0 gates native questions in Default mode behind this
        # opt-in. Keep coding in Default and use the existing native bridge.
        command += ["-c", f'model_providers.{provider}.env_key="SEVEN_PROVIDER_TOKEN"',
                    "-c", "features.default_mode_request_user_input=true"]
    schema = None
    if mode == "REVIEW":
        schema_path, output = _path(inputs["output_schema"]), _path(inputs["output_path"])
        info = schema_path.lstat()
        if not stat.S_ISREG(info.st_mode) or not 0 < info.st_size <= 256 << 10 or os.path.lexists(output):
            raise AdapterFailure("CODEX_REVIEW_INPUT_INVALID")
        try:
            schema = json.loads(schema_path.read_text())
        except (OSError, ValueError, UnicodeError, RecursionError):
            raise AdapterFailure("CODEX_REVIEW_SCHEMA_INVALID") from None
    return command, env, workspace, prompt, model, provider, schema, operation_tool_result, tuple(frozen_tools)


runtime = CodexAppServer(safe_text=safe_assistant_text)
adapter = AgentAdapter(name="codex", version="0.147.0",
                       capabilities=("interaction.request", "tool.request"))


async def execute(request, context, session_reference=""):
    command, env, cwd, prompt, model, provider, schema, operation_tool_result, frozen_tools = prepare(
        request.inputs, session_reference)
    for key in ("HOME", "CODEX_HOME"):
        Path(env[key]).mkdir(mode=0o700, parents=True, exist_ok=True)
    try:
        result = await runtime.run(request, context, command=command, env=env, cwd=cwd, prompt=prompt,
                                   model=model, provider=provider, output_schema=schema,
                                   session_reference=session_reference,
                                   operation_tool_result=operation_tool_result,
                                   available_tools=frozen_tools)
    except AppServerFailure as error:
        raise AdapterFailure(str(error)) from None
    finally:
        # A completed native turn is a platform safe point.  Reap App Server
        # here so SQLite/session files are quiescent before Processor and
        # Checkpoint readback.  A later round starts a fresh process and uses
        # thread/resume; it must not depend on an old process remaining alive.
        await runtime.shutdown()
    if request.inputs["mode"] == "REVIEW":
        output = Path(request.inputs["output_path"])
        if not runtime.final_message:
            raise AdapterFailure("CODEX_REVIEW_REPORT_MISSING")
        _write_review_report(output, runtime.final_message)
    return result


@adapter.run
async def run(request, context):
    return await execute(request, context)


@adapter.resume
async def resume(request, context):
    return await execute(request, context, request.session_reference)


@adapter.message
async def message(value, context):
    return await runtime.steer(value, context)


@adapter.suspend
async def suspend(request, context):
    return await runtime.suspend()


if __name__ == "__main__":
    adapter.serve()
