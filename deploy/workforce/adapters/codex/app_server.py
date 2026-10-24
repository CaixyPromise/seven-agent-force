"""Bounded Codex 0.147.0 App Server client used only inside the image Adapter."""
from __future__ import annotations

import asyncio
import hashlib
import json
from pathlib import Path
import re

from seven_agent.force.agent import ActivityStatus, RoundOutcome, RoundResult
from seven_agent.force.interaction import DeliveryStatus, Question, QuestionOption, QuestionRequest
from seven_agent.force.tools import (ToolCallRequest, ToolDefinition, ToolDeliveryStatus,
                                            ToolResult)

CODEX_EXECUTABLE = "/usr/local/bin/codex"
BASE_URL = "http://127.0.0.1:8787/v1"
MAX_LINE = 1 << 20
MAX_PENDING = 256
ROUND_TIMEOUT = 3600
NETWORK_TOOL_NAME = "run_command_with_network_access"
GIT_DELIVERY_TOOL_NAME = "request_git_delivery"
NETWORK_TOOL_DESCRIPTION = "Run a shell command that requires outbound network access. Call this tool whenever the task container cannot reach a required external host; do not retry the command through the normal shell while this call is pending. Provide the purpose, exact shell command, repository-relative working directory, and every required host, protocol, and port. The platform may ask the user to allow this operation once or for the rest of the task."
NETWORK_TOOL = ToolDefinition(NETWORK_TOOL_NAME, NETWORK_TOOL_DESCRIPTION, {
    "type": "object",
    "additionalProperties": False,
    "required": ["purpose", "command", "cwd", "targets"],
    "properties": {
        "purpose": {"type": "string", "minLength": 1, "maxLength": 2048},
        "command": {"type": "string", "minLength": 1, "maxLength": 16384},
        "cwd": {"type": "string", "minLength": 1, "maxLength": 4096},
        "targets": {
            "type": "array", "minItems": 1, "maxItems": 8,
            "items": {
                "type": "object", "additionalProperties": False,
                "required": ["host", "protocol", "port"],
                "properties": {
                    "host": {"type": "string", "minLength": 1, "maxLength": 253},
                    "protocol": {"type": "string", "enum": ["http", "https"]},
                    "port": {"type": "integer", "minimum": 1, "maximum": 65535},
                },
            },
        },
    },
})
GIT_DELIVERY_TOOL_DESCRIPTION = "Request delivery of the finished and validated task result to the task's configured Git destination. Call this only after implementation and required checks are complete. The platform freezes the result, completes required review, and either uses the task's existing delivery authorization or asks the user to decide. This tool never exposes Git credentials and does not push directly."
GIT_DELIVERY_TOOL = ToolDefinition(GIT_DELIVERY_TOOL_NAME, GIT_DELIVERY_TOOL_DESCRIPTION, {
    "type": "object",
    "additionalProperties": False,
    "required": ["summary"],
    "properties": {
        "summary": {"type": "string", "minLength": 1, "maxLength": 2048},
    },
})
PLATFORM_TOOLS = {
    NETWORK_TOOL_NAME: NETWORK_TOOL,
    GIT_DELIVERY_TOOL_NAME: GIT_DELIVERY_TOOL,
}
TOOL_PROMPT_PREFIX = "Native platform tools registered for this turn: {names}. Invoke these through native tool calling; do not search for an executable or shell command with the same name. Follow each tool description and schema exactly."


class AppServerFailure(ValueError):
    """Only constant machine codes may cross the Adapter boundary."""


def _restored_rollout_path(codex_home, session_reference):
    """Resume the copied rollout rather than an obsolete absolute index path.

    Native Codex owns the index and updates it through thread/resume. Never
    rewrite its SQLite store or mount the preceding Attempt's state. The
    response identity is checked before any continuation turn is started.
    """
    if not re.fullmatch(r"[A-Za-z0-9_-]{1,512}", session_reference):
        raise AppServerFailure("CODEX_SESSION_INVALID")
    root = Path(codex_home).resolve()
    try:
        candidates = list((root / "sessions").rglob("rollout-*-" + session_reference + ".jsonl"))
        if not candidates:
            return None
        if len(candidates) != 1:
            raise AppServerFailure("CODEX_SESSION_ROLLOUT_INVALID")
        path = candidates[0]
        path.resolve(strict=True).relative_to(root)
        if path.is_symlink() or not path.is_file():
            raise AppServerFailure("CODEX_SESSION_ROLLOUT_INVALID")
        return str(path.resolve(strict=True))
    except (OSError, ValueError):
        raise AppServerFailure("CODEX_SESSION_ROLLOUT_INVALID") from None


def _safe_key(value: object, prefix: str) -> str:
    if isinstance(value, str) and re.fullmatch(r"[A-Za-z0-9._:-]{1,96}", value):
        return prefix + value
    digest = hashlib.sha256(str(value).encode("utf-8", errors="replace")).hexdigest()[:32]
    return prefix + digest


def _text_input(text: str) -> list[dict]:
    return [{"type": "text", "text": text}]


def _prompt_with_tools(prompt: str, tools: tuple[ToolDefinition, ...]) -> str:
    if not tools:
        return prompt
    names = ", ".join(tool.name for tool in tools)
    return TOOL_PROMPT_PREFIX.format(names=names) + "\n\n" + prompt


def _repository_relative_arguments(arguments, workspace):
    """Normalize Codex-native absolute cwd without weakening the platform contract.

    Code mode commonly supplies the current repository as an absolute container
    path even though the platform tool contract is repository-relative.  Only an
    absolute path proven to be inside this frozen workspace is translated; every
    other path is left to the strict SDK validator (and therefore rejected).
    """
    if not isinstance(arguments, dict):
        return arguments
    cwd = arguments.get("cwd")
    if not isinstance(cwd, str) or not Path(cwd).is_absolute():
        return arguments
    try:
        root = Path(workspace).resolve(strict=True)
        relative = Path(cwd).resolve(strict=False).relative_to(root)
    except (OSError, ValueError):
        raise AppServerFailure("CODEX_DYNAMIC_TOOL_INPUT_INVALID") from None
    normalized = dict(arguments)
    normalized["cwd"] = relative.as_posix() if relative.parts else "."
    return normalized


def _tool_request(call_id, tool_name, arguments, workspace):
    try:
        if tool_name == NETWORK_TOOL_NAME:
            arguments = _repository_relative_arguments(arguments, workspace)
        return ToolCallRequest(str(call_id), _safe_key(call_id, "native-tool:"),
                               tool_name, arguments)
    except Exception:
        raise AppServerFailure("CODEX_DYNAMIC_TOOL_INPUT_INVALID") from None


def _native_tool_result(result):
    if not isinstance(result, ToolResult):
        raise AppServerFailure("CODEX_DYNAMIC_TOOL_RESULT_INVALID")
    value = result.wire()
    value.pop("localToolCallKey")
    value.pop("nativeToolCallId")
    return {"success": result.status in {"SUCCEEDED", "SCHEDULED"},
            "contentItems": [{"type": "inputText", "text": json.dumps(
                value, ensure_ascii=True, sort_keys=True, separators=(",", ":"))}]}


def _operation_injection_item(result):
    """Build the exact raw Responses item accepted by Codex 0.147.0."""
    if not isinstance(result, ToolResult):
        raise AppServerFailure("CODEX_OPERATION_RESTORE_INVALID")
    try:
        value = ToolResult.from_wire(result.wire()).wire()
    except Exception:
        raise AppServerFailure("CODEX_OPERATION_RESTORE_INVALID") from None
    return {
        "type": "custom_tool_call_output",
        "call_id": result.native_tool_call_id,
        "name": NETWORK_TOOL_NAME,
        "output": [{
            # Raw Responses history uses snake_case input_text. This is
            # intentionally different from DynamicToolCallResponse's
            # camelCase inputText content item.
            "type": "input_text",
            "text": json.dumps(value, ensure_ascii=True, sort_keys=True,
                               separators=(",", ":")),
        }],
    }


OPERATION_CONTINUATION_TEXT = (
    "Continue after the previously requested network operation. "
    "Its platform-owned result was restored separately in the thread history."
)


class AppServerClient:
    """Minimal JSON-RPC peer: one reader, serialized writes, bounded requests."""

    def __init__(self, command, env, cwd, notification, server_request):
        self.command, self.env, self.cwd = command, env, cwd
        self.notification, self.server_request = notification, server_request
        self.process = None
        self._reader = self._stderr = None
        self._write_lock = asyncio.Lock()
        self._pending = {}
        self._sequence = 0
        self._tasks = set()
        self._closed = False

    async def start(self):
        self.process = await asyncio.create_subprocess_exec(
            *self.command, cwd=self.cwd, env=self.env,
            stdin=asyncio.subprocess.PIPE, stdout=asyncio.subprocess.PIPE,
            stderr=asyncio.subprocess.PIPE,
        )
        self._reader = asyncio.create_task(self._read())
        self._stderr = asyncio.create_task(self._drain_stderr())
        await self.request("initialize", {"clientInfo": {"name": "seven-workforce-adapter", "version": "1"},
                                          "capabilities": {"experimentalApi": True}})
        await self.notify("initialized", {})
        return self

    def _spawn(self, coroutine):
        task = asyncio.create_task(coroutine)
        self._tasks.add(task)
        task.add_done_callback(self._task_done)

    def _task_done(self, task):
        self._tasks.discard(task)
        if not task.cancelled():
            # Consume failures so a malformed native request cannot produce an
            # unobserved-task warning or leak exception details to stderr.
            task.exception()

    async def _read(self):
        try:
            while True:
                raw = await self.process.stdout.readline()
                if not raw:
                    raise AppServerFailure("CODEX_APP_SERVER_DISCONNECTED")
                if len(raw) > MAX_LINE:
                    raise AppServerFailure("CODEX_APP_SERVER_FRAME_LIMIT")
                try:
                    frame = json.loads(raw)
                except (ValueError, UnicodeError, RecursionError):
                    raise AppServerFailure("CODEX_APP_SERVER_FRAME_INVALID") from None
                if not isinstance(frame, dict):
                    raise AppServerFailure("CODEX_APP_SERVER_FRAME_INVALID")
                if "id" in frame and "method" not in frame:
                    future = self._pending.pop(str(frame["id"]), None)
                    if future is not None and not future.done():
                        if "error" in frame:
                            future.set_exception(AppServerFailure("CODEX_APP_SERVER_REQUEST_FAILED"))
                        else:
                            future.set_result(frame.get("result"))
                    continue
                method, params = frame.get("method"), frame.get("params", {})
                if not isinstance(method, str) or not isinstance(params, dict):
                    raise AppServerFailure("CODEX_APP_SERVER_FRAME_INVALID")
                if "id" in frame:
                    self._spawn(self._dispatch_server_request(frame["id"], method, params))
                    # Let the request handler register its stable platform
                    # occurrence before later native notifications can close it.
                    await asyncio.sleep(0)
                else:
                    await self.notification(method, params)
        except BaseException as error:
            code = error if isinstance(error, AppServerFailure) else AppServerFailure("CODEX_APP_SERVER_DISCONNECTED")
            for future in tuple(self._pending.values()):
                if not future.done():
                    future.set_exception(code)
            self._pending.clear()

    async def _dispatch_server_request(self, request_id, method, params):
        try:
            await self.server_request(request_id, method, params)
        except asyncio.CancelledError:
            raise
        except Exception:
            try:
                await self.respond(request_id, error={"code": -32603, "message": "native request handling failed"})
            except Exception:
                return

    async def _drain_stderr(self):
        total = 0
        while True:
            chunk = await self.process.stderr.read(16384)
            if not chunk:
                return
            total += len(chunk)
            if total > 32 << 20:
                await self.close()
                return

    async def _write(self, frame):
        if self._closed or self.process is None or self.process.returncode is not None:
            raise AppServerFailure("CODEX_APP_SERVER_DISCONNECTED")
        raw = json.dumps(frame, sort_keys=True, separators=(",", ":"), ensure_ascii=True).encode() + b"\n"
        if len(raw) > MAX_LINE:
            raise AppServerFailure("CODEX_APP_SERVER_FRAME_LIMIT")
        async with self._write_lock:
            self.process.stdin.write(raw)
            await asyncio.wait_for(self.process.stdin.drain(), 5)

    async def request(self, method, params, timeout=30):
        if len(self._pending) >= MAX_PENDING:
            raise AppServerFailure("CODEX_APP_SERVER_PENDING_LIMIT")
        self._sequence += 1
        request_id = self._sequence
        future = asyncio.get_running_loop().create_future()
        self._pending[str(request_id)] = future
        try:
            await self._write({"id": request_id, "method": method, "params": params})
            return await asyncio.wait_for(future, timeout)
        finally:
            self._pending.pop(str(request_id), None)

    async def notify(self, method, params):
        await self._write({"method": method, "params": params})

    async def respond(self, request_id, *, result=None, error=None):
        frame = {"id": request_id}
        frame["error" if error is not None else "result"] = error if error is not None else result
        await self._write(frame)

    async def close(self):
        if self._closed:
            return
        self._closed = True
        if self.process is not None and self.process.returncode is None:
            try:
                self.process.terminate()
            except ProcessLookupError:
                pass
            try:
                await asyncio.wait_for(self.process.wait(), 2)
            except asyncio.TimeoutError:
                try:
                    self.process.kill()
                except ProcessLookupError:
                    pass
                await self.process.wait()
        for task in tuple(self._tasks):
            task.cancel()
        if self._tasks:
            await asyncio.gather(*self._tasks, return_exceptions=True)
        for task in (self._reader, self._stderr):
            if task is not None and task is not asyncio.current_task():
                task.cancel()
        await asyncio.gather(*(task for task in (self._reader, self._stderr) if task is not None and task is not asyncio.current_task()), return_exceptions=True)


class CodexAppServer:
    """Maps native Codex identities and events into one active Adapter round."""

    def __init__(self, *, safe_text):
        self.safe_text = safe_text
        self.client = None
        self.context = None
        self.mode = ""
        self.thread_id = ""
        self.turn_id = ""
        self.turn_done = None
        self.turn_status = ""
        self.final_message = ""
        self.native_requests = {}
        self.native_tools = {}
        self.native_tool_requests = {}
        self.available_tools = {}
        self.resolved_requests = set()
        self._lifecycle_lock = asyncio.Lock()

    async def _notification(self, method, params):
        if self.context is None:
            return
        if method == "turn/started":
            turn = params.get("turn", {})
            if params.get("threadId") == self.thread_id and isinstance(turn, dict):
                self.turn_id = str(turn.get("id", ""))
            return
        if method == "turn/completed":
            turn = params.get("turn", {})
            completed_id = str(turn.get("id", "")) if isinstance(turn, dict) else ""
            if params.get("threadId") == self.thread_id and completed_id and (not self.turn_id or completed_id == self.turn_id):
                self.turn_id = completed_id
                self.turn_status = str(turn.get("status", "failed"))
                if self.turn_done is not None and not self.turn_done.done():
                    self.turn_done.set_result(None)
            return
        if method == "serverRequest/resolved":
            request_id = str(params.get("requestId", params.get("id", "")))
            tool_call = self.native_tool_requests.get(request_id)
            if tool_call is not None:
                native = self.native_tools.get(tool_call)
                # A resolved request is a withdrawal only while no platform
                # result has been returned. Native acceptance is established
                # separately by the dynamicToolCall item/completed event.
                if native is not None and native[1] is None and not native[2]:
                    native[2] = True
                    await self.context.close_tool(native[0], reason="NATIVE_REQUEST_CLEARED")
                return
            native = self.native_requests.get(request_id)
            if native is not None and not native[1]:
                native[1] = True
                await self.context.close_interaction(native[0], reason="NATIVE_REQUEST_CLEARED")
            elif native is None:
                if len(self.resolved_requests) >= MAX_PENDING:
                    raise AppServerFailure("CODEX_APP_SERVER_PENDING_LIMIT")
                self.resolved_requests.add(request_id)
            return
        if params.get("threadId") not in (None, self.thread_id) or params.get("turnId") not in (None, self.turn_id):
            return
        if method == "turn/plan/updated":
            steps = []
            for step in params.get("plan", []):
                if isinstance(step, dict) and isinstance(step.get("step"), str):
                    steps.append(self.safe_text(step["step"]))
            if steps:
                await self.context.emit_plan(steps[:32])
            return
        if method not in ("item/started", "item/completed"):
            return
        item = params.get("item")
        if not isinstance(item, dict):
            return
        kind, item_id = item.get("type"), item.get("id")
        if kind == "dynamicToolCall" and isinstance(item_id, str):
            native = self.native_tools.get(item_id)
            if method == "item/completed" and native is not None and native[1] is not None and not native[2]:
                native[2] = True
                await self.context.tool_received(native[1], status=ToolDeliveryStatus.RECEIVED)
            return
        if kind == "agentMessage" and method == "item/completed":
            text = item.get("text")
            if isinstance(text, str):
                self.final_message = text
                if self.mode == "AGENT":
                    await self.context.emit_message(self.safe_text(text))
            return
        titles = {"commandExecution": "Run command", "fileChange": "Update files",
                  "mcpToolCall": "Use tool", "webSearch": "Search web"}
        if kind not in titles or not isinstance(item_id, str):
            return
        status = ActivityStatus.STARTED
        if method == "item/completed":
            native_status = item.get("status")
            status = ActivityStatus.FAILED if native_status in ("failed", "declined") else ActivityStatus.SUCCEEDED
        await self.context.emit_activity(key=_safe_key(item_id, "codex:"), title=titles[kind], status=status)

    async def _server_request(self, request_id, method, params):
        if str(request_id) in self.resolved_requests:
            return
        if method in ("item/commandExecution/requestApproval", "item/fileChange/requestApproval", "permissions/requestApproval"):
            await self.client.respond(request_id, error={"code": -32001, "message": "platform authorization unavailable"})
            return
        if method == "item/tool/call" and self.context is not None and self.mode == "AGENT":
            await self._dynamic_tool_call(request_id, params)
            return
        if method != "item/tool/requestUserInput" or self.context is None or self.mode != "AGENT":
            await self.client.respond(request_id, error={"code": -32601, "message": "unsupported native request"})
            return
        raw_questions = params.get("questions")
        if not isinstance(raw_questions, list) or not 1 <= len(raw_questions) <= 3 or any(q.get("isSecret") for q in raw_questions if isinstance(q, dict)):
            await self.client.respond(request_id, error={"code": -32002, "message": "unsupported user input"})
            return
        questions, option_labels = [], {}
        try:
            for index, raw in enumerate(raw_questions):
                if not isinstance(raw, dict):
                    raise ValueError()
                key = str(raw.get("id", ""))
                options = raw.get("options")
                mapped = []
                if options is not None:
                    if not isinstance(options, list) or len(options) > 8:
                        raise ValueError()
                    for option_index, option in enumerate(options):
                        if not isinstance(option, dict):
                            raise ValueError()
                        option_key = f"o{index + 1}-{option_index + 1}"
                        label = str(option.get("label", ""))
                        mapped.append(QuestionOption(option_key, label))
                        option_labels[(key, option_key)] = label
                kind = "SINGLE_CHOICE" if mapped else "TEXT"
                questions.append(Question(key=key, title=str(raw.get("header", "Question")),
                                          description=str(raw.get("question", "")), kind=kind,
                                          required=True, options=tuple(mapped),
                                          allow_free_text=bool(raw.get("isOther", False) or not mapped)))
            interaction_key = _safe_key(params.get("itemId", request_id), "native:")
            request = QuestionRequest(interaction_key, tuple(questions), bool(params.get("isBlocking", True)))
            self.native_requests[str(request_id)] = [interaction_key, False]
            answers = await self.context.ask(request)
            native_answers = {}
            for answer in answers:
                values = [option_labels[(answer.question_key, key)] for key in answer.selected_option_keys]
                if answer.text:
                    values.append(answer.text)
                native_answers[answer.question_key] = {"answers": values}
            await self.client.respond(request_id, result={"answers": native_answers})
            self.native_requests[str(request_id)][1] = True
        except asyncio.CancelledError:
            if not self.native_requests.get(str(request_id), ["", True])[1]:
                await self.client.respond(request_id, error={"code": -32003, "message": "user input unavailable"})
            raise
        except Exception:
            if not self.native_requests.get(str(request_id), ["", True])[1]:
                await self.client.respond(request_id, error={"code": -32003, "message": "user input unavailable"})

    async def _dynamic_tool_call(self, request_id, params):
        call_id = str(params.get("callId", ""))
        tool_name = params.get("tool")
        try:
            if (params.get("threadId") != self.thread_id or params.get("turnId") != self.turn_id
                    or params.get("namespace") is not None or tool_name not in self.available_tools
                    or not call_id):
                raise AppServerFailure("CODEX_DYNAMIC_TOOL_INPUT_INVALID")
            request = _tool_request(call_id, tool_name, params.get("arguments"), self.client.cwd)
            previous = self.native_tools.get(call_id)
            if previous is not None and previous[3] != request.wire():
                raise AppServerFailure("CODEX_DYNAMIC_TOOL_IDENTITY_CONFLICT")
            if previous is None:
                previous = [request.local_tool_call_key, None, False, request.wire()]
                self.native_tools[call_id] = previous
            self.native_tool_requests[str(request_id)] = call_id
            result = previous[1]
            if result is None:
                result = await self.context.request_tool(request)
                previous[1] = result
            await self.client.respond(request_id, result=_native_tool_result(result))
        except asyncio.CancelledError:
            if call_id in self.native_tools and not self.native_tools[call_id][2]:
                self.native_tools[call_id][2] = True
                await self.context.close_tool(self.native_tools[call_id][0], reason="TURN_INTERRUPTED")
            raise
        except Exception:
            await self.client.respond(request_id, error={"code": -32004, "message": "dynamic tool unavailable"})

    async def _ensure(self, command, env, cwd):
        if self.client is None or self.client.process.returncode is not None:
            self.native_requests.clear()
            self.native_tools.clear()
            self.native_tool_requests.clear()
            self.resolved_requests.clear()
            self.client = await AppServerClient(command, env, cwd, self._notification, self._server_request).start()

    async def run(self, request, context, *, command, env, cwd, prompt, model, provider,
                  output_schema=None, session_reference="", operation_tool_result=None,
                  available_tools=()):
        async with self._lifecycle_lock:
            if operation_tool_result is not None:
                if request.inputs.get("mode") != "AGENT" or not session_reference or output_schema is not None:
                    raise AppServerFailure("CODEX_OPERATION_RESTORE_INVALID")
                injection_item = _operation_injection_item(operation_tool_result)
            else:
                injection_item = None
            self.context, self.mode = context, request.inputs["mode"]
            self.final_message, self.turn_status, self.turn_id = "", "", ""
            self.turn_done = asyncio.get_running_loop().create_future()
            await self._ensure(command, env, cwd)
            await context.started(native_pid=self.client.process.pid)
            thread_params = {"cwd": str(cwd), "model": model, "modelProvider": provider,
                             "approvalPolicy": "never",
                             # The outer Attempt/Reviewer container is the
                             # authoritative sandbox.  Starting Codex's inner
                             # Linux sandbox fails under the deliberately
                             # capability-free, no-new-privileges container
                             # and does not add a trust boundary there.
                             "sandbox": "danger-full-access"}
            if self.mode == "REVIEW":
                thread_params["ephemeral"] = True
            else:
                if (not isinstance(available_tools, tuple)
                        or any(not isinstance(tool, ToolDefinition) for tool in available_tools)):
                    raise AppServerFailure("CODEX_DYNAMIC_TOOL_UNAVAILABLE")
                self.available_tools = {tool.name: tool for tool in available_tools}
                if len(self.available_tools) != len(available_tools):
                    raise AppServerFailure("CODEX_DYNAMIC_TOOL_UNAVAILABLE")
                thread_params["dynamicTools"] = [tool.wire() for tool in available_tools]
            if session_reference:
                resume_params = {"threadId": session_reference, **thread_params}
                rollout_path = _restored_rollout_path(env["CODEX_HOME"], session_reference)
                if rollout_path is not None:
                    resume_params["path"] = rollout_path
                response = await self.client.request("thread/resume", resume_params)
            else:
                response = await self.client.request("thread/start", thread_params)
            thread = response.get("thread", {}) if isinstance(response, dict) else {}
            self.thread_id = str(thread.get("id", ""))
            if not self.thread_id:
                raise AppServerFailure("CODEX_SESSION_UNAVAILABLE")
            if session_reference and self.thread_id != session_reference:
                raise AppServerFailure("CODEX_SESSION_IDENTITY_MISMATCH")
            if self.mode == "AGENT" and available_tools:
                await context.tool_catalog_ready(tuple(tool.name for tool in available_tools))
            if injection_item is not None:
                try:
                    await self.client.request("thread/inject_items", {
                        "threadId": self.thread_id,
                        "items": [injection_item],
                    })
                except AppServerFailure:
                    # The item may have been committed before the response was
                    # lost. Never replay it or start a continuation turn from
                    # this Adapter invocation when the outcome is uncertain.
                    raise AppServerFailure("CODEX_OPERATION_INJECTION_OUTCOME_UNKNOWN") from None
                turn_params = {"threadId": self.thread_id,
                               "input": _text_input(OPERATION_CONTINUATION_TEXT)}
            else:
                turn_params = {"threadId": self.thread_id,
                               "input": _text_input(_prompt_with_tools(prompt, available_tools))}
            if output_schema is not None:
                turn_params["outputSchema"] = output_schema
            response = await self.client.request("turn/start", turn_params)
            turn = response.get("turn", {}) if isinstance(response, dict) else {}
            response_turn = str(turn.get("id", ""))
            if response_turn:
                self.turn_id = response_turn
            await asyncio.wait_for(self.turn_done, ROUND_TIMEOUT)
            for native in tuple(self.native_requests.values()):
                if not native[1]:
                    native[1] = True
                    await context.close_interaction(native[0], reason="TURN_COMPLETED")
            for native in tuple(self.native_tools.values()):
                if not native[2]:
                    native[2] = True
                    await context.close_tool(native[0], reason="TURN_COMPLETED")
            if self.turn_status not in ("completed", "succeeded"):
                return RoundResult(RoundOutcome.FAILED, "CODEX_EXECUTION_FAILED", self.thread_id)
            return RoundResult.finished(summary="Codex round finished", session_reference=self.thread_id)

    async def steer(self, message, context):
        if self.client is None or not self.thread_id or not self.turn_id or self.turn_done is None or self.turn_done.done():
            return DeliveryStatus.UNKNOWN
        try:
            await self.client.request("turn/steer", {"threadId": self.thread_id, "expectedTurnId": self.turn_id,
                                                     "input": _text_input(message.text)})
            return DeliveryStatus.RECEIVED
        except AppServerFailure:
            return DeliveryStatus.UNKNOWN

    async def suspend(self):
        if self.client is None or not self.thread_id:
            raise AppServerFailure("CODEX_SESSION_UNAVAILABLE")
        if self.turn_id and self.turn_done is not None and not self.turn_done.done():
            await self.client.request("turn/interrupt", {"threadId": self.thread_id, "turnId": self.turn_id})
            await asyncio.wait_for(self.turn_done, 10)
        return self.thread_id

    async def shutdown(self):
        if self.client is not None:
            await self.client.close()
            self.client = None
