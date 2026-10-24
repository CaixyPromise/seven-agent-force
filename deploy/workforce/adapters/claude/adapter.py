"""Image-owned Claude Code 2.1.273 Agent Adapter."""
from __future__ import annotations

import asyncio
import hashlib
import json
import os
from pathlib import Path
import re
import stat
import sys

sys.path.insert(0, "/opt/seven-worker/result-runtime")

from seven_agent.force.agent import AgentAdapter, RoundResult
from seven_agent.force.interaction import (DeliveryStatus, Question, QuestionOption,
                                                  QuestionRequest)
from seven_agent.force.tools import ToolCallRequest, ToolDefinition


CLAUDE = os.environ.get("SEVEN_CLAUDE_EXECUTABLE", "/usr/local/bin/claude")
BRIDGE = os.environ.get("SEVEN_CLAUDE_MCP_BRIDGE", "/opt/seven/adapter/mcp_bridge.py")
FACADE = "http://127.0.0.1:8787"
LIMIT = 1 << 20
ASK_USER = "ask_user"
HOOK_ENVIRONMENT = ("SEVEN_TASK_ID", "SEVEN_ATTEMPT_ID", "SEVEN_ATTEMPT_GENERATION",
                    "SEVEN_CONTROL_SOCKET", "SEVEN_HOOK_CONFIGURATION_ROOT",
                    "SEVEN_HOOK_CONFIGURATION_DIGEST")


class ClaudeFailure(ValueError):
    pass


def _text(value, limit, *, nonempty=True):
    if (not isinstance(value, str) or "\0" in value or (nonempty and not value.strip())
            or len(value.encode("utf-8", errors="replace")) > limit):
        raise ClaudeFailure("CLAUDE_INPUT_INVALID")
    return value


def _path(value):
    value = _text(value, 4096)
    result = Path(value)
    if not result.is_absolute() or ".." in result.parts or any(ord(c) < 32 for c in value):
        raise ClaudeFailure("CLAUDE_INPUT_INVALID")
    return result


def _model(value):
    value = _text(value, 256)
    if value != value.strip() or any(ord(c) < 32 or ord(c) == 127 for c in value):
        raise ClaudeFailure("CLAUDE_MODEL_INVALID")
    return value


def _tools(value):
    if not isinstance(value, list) or len(value) > 16:
        raise ClaudeFailure("CLAUDE_DYNAMIC_TOOL_UNAVAILABLE")
    result, names = [], set()
    for item in value:
        if not isinstance(item, dict) or set(item) != {"name", "description", "inputSchema"}:
            raise ClaudeFailure("CLAUDE_DYNAMIC_TOOL_UNAVAILABLE")
        definition = ToolDefinition(item["name"], item["description"], item["inputSchema"])
        wire = definition.wire()
        if definition.name in names:
            raise ClaudeFailure("CLAUDE_DYNAMIC_TOOL_UNAVAILABLE")
        names.add(definition.name)
        result.append((definition, {"name": wire["name"], "description": wire["description"],
                                    "inputSchema": wire["inputSchema"]}))
    return tuple(result)


def _ask_definition():
    option = {
        "type": "object", "additionalProperties": False,
        "required": ["key", "label"],
        "properties": {"key": {"type": "string"}, "label": {"type": "string"}},
    }
    question = {
        "type": "object", "additionalProperties": False,
        "required": ["key", "title"],
        "properties": {
            "key": {"type": "string"}, "title": {"type": "string"},
            "description": {"type": "string"},
            "options": {"type": "array", "maxItems": 8, "items": option},
        },
    }
    return {
        "name": ASK_USER,
        "description": "Ask the user one to three necessary questions and wait for their answers.",
        "inputSchema": {
            "type": "object", "additionalProperties": False,
            "required": ["interactionKey", "questions"],
            "properties": {
                "interactionKey": {"type": "string"},
                "questions": {"type": "array", "minItems": 1, "maxItems": 3,
                              "items": question},
            },
        },
    }


def _review_write(path, body):
    encoded = body.encode("utf-8")
    if not encoded or len(encoded) > 256 << 10 or os.path.lexists(path):
        raise ClaudeFailure("CLAUDE_REVIEW_REPORT_INVALID")
    flags = os.O_WRONLY | os.O_CREAT | os.O_EXCL | getattr(os, "O_NOFOLLOW", 0)
    descriptor = os.open(path, flags, 0o600)
    with os.fdopen(descriptor, "wb") as stream:
        stream.write(encoded)
        stream.flush()
        os.fsync(stream.fileno())


def _hook_environment(inputs):
    values = {key: os.environ[key] for key in HOOK_ENVIRONMENT if key in os.environ}
    if not values:
        return values
    if not set(HOOK_ENVIRONMENT[:4]) <= values.keys():
        raise ClaudeFailure("CLAUDE_HOOK_METADATA_INVALID")
    for key in HOOK_ENVIRONMENT[:3]:
        if re.fullmatch(r"[1-9][0-9]{0,18}", values[key]) is None or int(values[key]) > (1 << 63) - 1:
            raise ClaudeFailure("CLAUDE_HOOK_METADATA_INVALID")
    if values["SEVEN_TASK_ID"] != str(inputs["taskId"]) or values["SEVEN_ATTEMPT_ID"] != str(inputs["attemptId"]):
        raise ClaudeFailure("CLAUDE_HOOK_METADATA_INVALID")
    socket = values["SEVEN_CONTROL_SOCKET"]
    if not socket.startswith("/") or Path(socket).name != "s" or any(part in ("", ".", "..") for part in socket.split("/")[1:]):
        raise ClaudeFailure("CLAUDE_HOOK_METADATA_INVALID")
    root, digest = values.get("SEVEN_HOOK_CONFIGURATION_ROOT"), values.get("SEVEN_HOOK_CONFIGURATION_DIGEST")
    if root is not None or digest is not None:
        if digest is None or re.fullmatch(r"[0-9a-f]{64}", digest) is None or root != "/opt/seven-worker/managed-hooks/" + digest:
            raise ClaudeFailure("CLAUDE_HOOK_METADATA_INVALID")
    return values


def _tool_identity(native_call_id, name):
    if (not isinstance(native_call_id, str) or not native_call_id
            or len(native_call_id.encode("utf-8")) > 256
            or any(ord(character) < 32 or ord(character) == 127 for character in native_call_id)):
        raise ClaudeFailure("CLAUDE_MCP_REQUEST_INVALID")
    occurrence = hashlib.sha256((name + "\0" + native_call_id).encode("utf-8")).hexdigest()
    return "claude-mcp-" + occurrence, "claude:" + occurrence[:32]


class Runtime:
    def __init__(self):
        self.process = None
        self.session_reference = ""
        self.input_lock = asyncio.Lock()
        self.accepting_messages = False

    async def write_message(self, text):
        async with self.input_lock:
            process = self.process
            if (not self.accepting_messages or process is None or process.returncode is not None
                    or process.stdin is None or process.stdin.is_closing()):
                return DeliveryStatus.UNKNOWN
            body = {"type": "user", "message": {"role": "user", "content": text}}
            try:
                process.stdin.write((json.dumps(body, separators=(",", ":"), ensure_ascii=False) + "\n").encode())
                await process.stdin.drain()
                return DeliveryStatus.RECEIVED
            except (BrokenPipeError, ConnectionResetError):
                self.accepting_messages = False
                return DeliveryStatus.UNKNOWN

    async def close_input(self):
        async with self.input_lock:
            self.accepting_messages = False
            process = self.process
            if process is not None and process.stdin is not None and not process.stdin.is_closing():
                process.stdin.close()

    async def stop(self):
        await self.close_input()
        process = self.process
        if process is not None and process.returncode is None:
            process.terminate()
            try:
                await asyncio.wait_for(process.wait(), 5)
            except asyncio.TimeoutError:
                process.kill()
                await process.wait()


runtime = Runtime()
adapter = AgentAdapter(name="claude-code", version="2.1.273",
                       capabilities=("interaction.request", "tool.request"))


def prepare(inputs, session_reference=""):
    if not isinstance(inputs, dict):
        raise ClaudeFailure("CLAUDE_INPUT_INVALID")
    mode = inputs.get("mode")
    if mode == "AGENT":
        required = {"mode", "taskId", "attemptId", "instruction", "workspace", "output", "state",
                    "temporary", "provider", "feedback", "availableTools"}
        optional = {"reviewCorrection", "interactionQuestionRequest", "interactionResponse", "operationToolResult"}
        if not required <= set(inputs) or set(inputs) - required - optional:
            raise ClaudeFailure("CLAUDE_INPUT_INVALID")
        if any(re.fullmatch(r"[1-9][0-9]{0,18}", str(inputs[key])) is None for key in ("taskId", "attemptId")):
            raise ClaudeFailure("CLAUDE_INPUT_INVALID")
        provider = inputs["provider"]
        if not isinstance(provider, dict) or set(provider) != {"model"}:
            raise ClaudeFailure("CLAUDE_INPUT_INVALID")
        model = _model(provider["model"])
        prompt = _text(inputs["feedback"], 64 << 10, nonempty=False) if session_reference else _text(inputs["instruction"], 64 << 10)
        tools = _tools(inputs["availableTools"])
        output_schema = output_path = None
        state = _path(inputs["state"])
        hook_environment = _hook_environment(inputs)
    elif mode == "REVIEW":
        required = {"mode", "workspace", "model", "prompt", "output_schema", "output_path"}
        optional = {"resultCommit", "resultTree", "requirements_path", "review_contract_path"}
        if not required <= set(inputs) or set(inputs) - required - optional:
            raise ClaudeFailure("CLAUDE_INPUT_INVALID")
        model, prompt, tools = _model(inputs["model"]), _text(inputs["prompt"], 64 << 10), ()
        output_schema, output_path = _path(inputs["output_schema"]), _path(inputs["output_path"])
        state = Path("/tmp/seven-claude-review")
        hook_environment = {}
    else:
        raise ClaudeFailure("CLAUDE_INPUT_INVALID")
    workspace = _path(inputs["workspace"])
    if session_reference and re.fullmatch(r"[A-Za-z0-9_-]{1,512}", session_reference) is None:
        raise ClaudeFailure("CLAUDE_SESSION_INVALID")
    return mode, workspace, state, model, prompt, tools, output_schema, output_path, hook_environment


async def _serve_tool(reader, writer, context, tools_by_name):
    try:
        raw = await reader.readline()
        if not raw or len(raw) > 128 << 10:
            raise ClaudeFailure("CLAUDE_MCP_REQUEST_INVALID")
        call = json.loads(raw)
        if not isinstance(call, dict) or set(call) != {"nativeCallId", "name", "arguments"}:
            raise ClaudeFailure("CLAUDE_MCP_REQUEST_INVALID")
        native_call_id, name, arguments = call["nativeCallId"], call["name"], call["arguments"]
        if not isinstance(arguments, dict):
            raise ClaudeFailure("CLAUDE_MCP_REQUEST_INVALID")
        if name == ASK_USER:
            questions = []
            for item in arguments.get("questions", []):
                options = tuple(QuestionOption(option["key"], option["label"])
                                for option in item.get("options", []))
                kind = "SINGLE_CHOICE" if options else "TEXT"
                questions.append(Question(item["key"], item["title"], item.get("description", ""),
                                          kind, True, not options, options))
            answers = await context.ask(QuestionRequest(arguments["interactionKey"], tuple(questions)))
            value = {"answers": [answer.wire() for answer in answers]}
        else:
            if name not in tools_by_name:
                raise ClaudeFailure("CLAUDE_DYNAMIC_TOOL_UNAVAILABLE")
            native_id, key = _tool_identity(native_call_id, name)
            result = await context.request_tool(ToolCallRequest(native_id, key, name, arguments))
            await context.tool_received(result)
            value = result.wire()
        response = {"ok": True, "value": value}
    except Exception as error:
        response = {"ok": False, "value": str(error)[:256]}
    writer.write(json.dumps(response, separators=(",", ":"), ensure_ascii=False).encode() + b"\n")
    await writer.drain()
    writer.close()
    await writer.wait_closed()


def _extract(event):
    session = event.get("session_id") or event.get("sessionId") or ""
    texts = []
    message = event.get("message")
    if isinstance(message, dict):
        for content in message.get("content", []):
            if isinstance(content, dict) and content.get("type") == "text" and isinstance(content.get("text"), str):
                texts.append(content["text"])
    if event.get("type") == "result" and isinstance(event.get("result"), str):
        texts.append(event["result"])
    structured = event.get("structured_output") or event.get("structuredOutput")
    return session, texts, structured


async def execute(request, context, session_reference=""):
    mode, cwd, state, model, prompt, tools, schema_path, output_path, hook_environment = prepare(request.inputs, session_reference)
    state.mkdir(mode=0o700, parents=True, exist_ok=True)
    socket_path = state / ("mcp-" + request.round_key + ".sock")
    catalog_path = state / ("mcp-" + request.round_key + ".json")
    mcp_config_path = state / ("mcp-config-" + request.round_key + ".json")
    catalog = [wire for _, wire in tools]
    if mode == "AGENT":
        catalog.append(_ask_definition())
    catalog_path.write_text(json.dumps(catalog, separators=(",", ":")), encoding="utf-8")
    os.chmod(catalog_path, 0o600)
    env = {"PATH": "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
           "HOME": str(state / "home"), "CLAUDE_CONFIG_DIR": str(state / "config"),
           "ANTHROPIC_BASE_URL": FACADE, "ANTHROPIC_AUTH_TOKEN": "seven-local-provider-facade",
           "DISABLE_AUTOUPDATER": "1", "ENABLE_TOOL_SEARCH": "false",
           "SEVEN_CLAUDE_MCP_SOCKET": str(socket_path), "SEVEN_CLAUDE_MCP_CATALOG": str(catalog_path),
           "LANG": "C.UTF-8"}
    env.update(hook_environment)
    Path(env["HOME"]).mkdir(mode=0o700, parents=True, exist_ok=True)
    Path(env["CLAUDE_CONFIG_DIR"]).mkdir(mode=0o700, parents=True, exist_ok=True)
    if hook_environment.get("SEVEN_HOOK_CONFIGURATION_DIGEST"):
        hook_command = "/usr/local/bin/seven-hook dispatch --event "
        settings = {"hooks": {
            "PreToolUse": [{"hooks": [{"type": "command", "command": hook_command + "claude.PreToolUse"}]}],
            "PostToolUse": [{"hooks": [{"type": "command", "command": hook_command + "claude.PostToolUse"}]}],
        }}
        settings_path = Path(env["CLAUDE_CONFIG_DIR"]) / "settings.json"
        settings_path.write_text(json.dumps(settings, separators=(",", ":")), encoding="utf-8")
        os.chmod(settings_path, 0o600)
    config = {"mcpServers": {"seven_platform": {"command": sys.executable, "args": [BRIDGE],
                                                   "env": {"SEVEN_CLAUDE_MCP_SOCKET": str(socket_path),
                                                           "SEVEN_CLAUDE_MCP_CATALOG": str(catalog_path)}}}}
    mcp_config_path.write_text(json.dumps(config, separators=(",", ":")), encoding="utf-8")
    os.chmod(mcp_config_path, 0o600)
    tools_by_name = {definition.name: definition for definition, _ in tools}
    server = await asyncio.start_unix_server(lambda r, w: _serve_tool(r, w, context, tools_by_name), str(socket_path))
    command = [CLAUDE, "--print", "--input-format", "stream-json", "--output-format", "stream-json",
               "--verbose", "--model", model, "--mcp-config", str(mcp_config_path)]
    if session_reference:
        command += ["--resume", session_reference]
    if mode == "REVIEW":
        schema = json.loads(schema_path.read_text(encoding="utf-8"))
        command += ["--json-schema", json.dumps(schema, separators=(",", ":"))]
    final_text, last_emitted_text, structured = "", "", None
    try:
        runtime.process = await asyncio.create_subprocess_exec(*command, cwd=cwd, env=env,
                                                               stdin=asyncio.subprocess.PIPE,
                                                               stdout=asyncio.subprocess.PIPE,
                                                               stderr=asyncio.subprocess.PIPE)
        await context.started(native_pid=runtime.process.pid)
        if mode == "AGENT":
            await context.tool_catalog_ready(tuple(tools_by_name))
        runtime.accepting_messages = True
        if await runtime.write_message(prompt) is not DeliveryStatus.RECEIVED:
            raise ClaudeFailure("CLAUDE_INPUT_UNAVAILABLE")
        while True:
            raw = await runtime.process.stdout.readline()
            if not raw:
                break
            if len(raw) > LIMIT:
                raise ClaudeFailure("CLAUDE_OUTPUT_TOO_LARGE")
            event = json.loads(raw)
            session, texts, candidate = _extract(event)
            if session:
                runtime.session_reference = session
            for text in texts:
                final_text = text
                if mode == "AGENT" and text != last_emitted_text:
                    await context.emit_message(text[:16384])
                    last_emitted_text = text
            if candidate is not None:
                structured = candidate
            if event.get("type") == "result":
                await runtime.close_input()
        code = await runtime.process.wait()
        if code != 0:
            raise ClaudeFailure("CLAUDE_EXECUTION_FAILED")
        if mode == "REVIEW":
            report = json.dumps(structured, separators=(",", ":"), ensure_ascii=False) if structured is not None else final_text
            _review_write(output_path, report)
        return RoundResult.finished(summary=final_text[:2048], session_reference=runtime.session_reference)
    finally:
        server.close()
        await server.wait_closed()
        await runtime.stop()
        for path in (socket_path, catalog_path, mcp_config_path):
            try:
                path.unlink()
            except FileNotFoundError:
                pass


@adapter.run
async def run(request, context):
    return await execute(request, context)


@adapter.resume
async def resume(request, context):
    return await execute(request, context, request.session_reference)


@adapter.message
async def message(value, context):
    return await runtime.write_message(value.text)


@adapter.suspend
async def suspend(request, context):
    await runtime.stop()
    if not runtime.session_reference:
        raise ClaudeFailure("CLAUDE_SESSION_UNAVAILABLE")
    return runtime.session_reference


if __name__ == "__main__":
    adapter.serve()
