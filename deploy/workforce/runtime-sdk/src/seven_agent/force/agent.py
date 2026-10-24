"""Agent-facing adapter objects, independent of platform business state."""
from __future__ import annotations

import asyncio
import dataclasses
import enum
import inspect
import hashlib
import json
import os
import sys
from collections import OrderedDict
from typing import Awaitable, Callable

from ._protocol import MAX_FRAME_BYTES, ProtocolError, VERSION, decode, encode
from .interaction import (DeliveryStatus, InteractionAnswer, InteractionResponse, Question, QuestionOption,
                          QuestionRequest, UserMessage, answers_from_wire, validate_answers,
                          text as interaction_text)
from .tools import (ToolCallRequest, ToolDefinition, ToolDeliveryStatus, ToolResult, ToolTarget)


class RoundOutcome(str, enum.Enum):
    FINISHED = "FINISHED"
    CANCELLED = "CANCELLED"
    FAILED = "FAILED"


class ActivityStatus(str, enum.Enum):
    STARTED = "STARTED"
    COMPLETED = "COMPLETED"
    SUCCEEDED = "SUCCEEDED"
    FAILED = "FAILED"


@dataclasses.dataclass(frozen=True)
class RunRequest:
    round_key: str
    inputs: dict


@dataclasses.dataclass(frozen=True)
class ResumeRequest(RunRequest):
    session_reference: str


@dataclasses.dataclass(frozen=True)
class RoundResult:
    outcome: RoundOutcome
    summary: str = ""
    session_reference: str = ""

    @classmethod
    def finished(cls, *, summary="", session_reference=""):
        return cls(RoundOutcome.FINISHED, summary, session_reference)


class ExecutionContext:
    def __init__(self, round_key, emit, *, capabilities=()):
        self.round_key = round_key
        self.cancelled = asyncio.Event()
        self._emit = emit
        self._started = False
        self._capabilities = frozenset(capabilities)
        self._questions = {}
        self._responses = {}
        self._restored_responses = {}
        self._tool_requests = {}
        self._tool_results = {}
        self._tool_receipt = None
        self._closed = False
        self._suspended = False
        self._suspending = False
        self._controls = set()
        self._control_unconfirmed = False

    def received_response(self, interaction_key: str) -> InteractionResponse | None:
        """Snapshot accepted delivery even before the ask coroutine is scheduled.

        This is local receipt, not persistence or application. Suspend handlers
        must persist this snapshot themselves before claiming native quiescence.
        """
        return self._responses.get(interaction_key)

    def _register_question(self, request, expected=None):
        if "interaction.request" not in self._capabilities:
            raise ProtocolError("ADAPTER_METHOD_UNSUPPORTED")
        if self.cancelled.is_set() or self._closed:
            raise asyncio.CancelledError()
        if self._suspending:
            raise ProtocolError("ADAPTER_ROUND_SUSPENDING")
        if not self._started or not isinstance(request, QuestionRequest):
            raise ProtocolError()
        payload = request.wire()
        previous = self._questions.get(request.interaction_key)
        if previous is not None:
            if previous[0] != payload or (expected is not None and
                    self._restored_responses.get(request.interaction_key) != expected):
                raise ProtocolError("ADAPTER_IDEMPOTENCY_CONFLICT")
            return payload, previous[1], False
        if len(self._questions) >= 256:
            raise ProtocolError("ADAPTER_SESSION_LIMIT")
        # Admission is atomic before the first await/emission. Native App Server
        # may keep several independent requests pending; retain a small fixed
        # bound and never evict a request that the host may already display.
        if sum(not future.done() for _, future in self._questions.values()) >= 8:
            raise ProtocolError("ADAPTER_INTERACTION_BUSY")
        future = asyncio.get_running_loop().create_future()
        self._questions[request.interaction_key] = (payload, future)
        if expected is not None:
            self._restored_responses[request.interaction_key] = expected
        return payload, future, True

    async def ask(self, request: QuestionRequest) -> tuple[InteractionAnswer, ...]:
        """Await the single pending keyed answer; same-key callers share it."""
        payload, future, created = self._register_question(request)
        if not created:
            return await asyncio.shield(future)
        try:
            await self._emit("interaction.request", {"roundKey": self.round_key, **payload})
            return await asyncio.shield(future)
        except BaseException:
            future.cancel()
            raise

    async def receive_restored_answer(self, question: QuestionRequest,
                                     expected_response: InteractionResponse) -> tuple[InteractionAnswer, ...]:
        """Wait for normal delivery of a saved response in this new round.

        Registers no visible request and generates no automatic receipt. Only a
        matching interaction.respond can resolve the wait and produce RECEIVED.
        The private snapshot alone is not accepted delivery or authority.
        """
        if not isinstance(question, QuestionRequest) or not isinstance(expected_response, InteractionResponse):
            raise ProtocolError()
        wire = expected_response.wire()
        if question.interaction_key != expected_response.interaction_key:
            raise ProtocolError("ADAPTER_IDEMPOTENCY_CONFLICT")
        validate_answers(question.wire(), answers_from_wire(wire["answers"]))
        _, future, created = self._register_question(question, encode(wire))
        try:
            return await asyncio.shield(future)
        except BaseException:
            if created:
                future.cancel()
            raise

    def _close(self):
        self._closed = True
        for _, future in self._questions.values():
            if not future.done():
                future.cancel()
        for _, future in self._tool_requests.values():
            if not future.done():
                future.cancel()

    async def started(self, *, native_pid: int | None = None):
        """Call after execution starts; omit PID only for an in-process agent."""
        if not self._started:
            payload = {"roundKey": self.round_key}
            if native_pid is None:
                native_pid = os.getpid()
            if type(native_pid) is not int or native_pid <= 0:
                raise ProtocolError()
            payload["nativePid"] = native_pid
            await self._emit("execution.started", payload)
            self._started = True

    async def emit_message(self, text: str):
        if not isinstance(text, str):
            raise ProtocolError()
        await self._emit("message", {"roundKey": self.round_key, "text": text})

    async def emit_plan(self, steps: list[str]):
        if not isinstance(steps, list) or any(not isinstance(step, str) for step in steps):
            raise ProtocolError()
        await self._emit("plan", {"roundKey": self.round_key, "steps": steps})

    async def emit_activity(self, *, key: str, title: str, status: ActivityStatus):
        if not isinstance(status, ActivityStatus) or not isinstance(key, str) or not isinstance(title, str):
            raise ProtocolError()
        await self._emit("activity", {"roundKey": self.round_key, "activityKey": key,
                                      "title": title, "status": status.value})

    async def tool_catalog_ready(self, tool_names: tuple[str, ...]):
        """Report the safe tool-name catalog accepted by the native runtime."""
        if "tool.request" not in self._capabilities or not isinstance(tool_names, tuple):
            raise ProtocolError("ADAPTER_METHOD_UNSUPPORTED")
        names = sorted(tool_names)
        body = json.dumps(names, ensure_ascii=True, separators=(",", ":")).encode("utf-8")
        await self._emit("tool.catalog", {"roundKey": self.round_key, "toolNames": names,
                                          "catalogDigest": hashlib.sha256(body).hexdigest()})

    async def close_interaction(self, interaction_key: str, *, reason: str):
        """Close a native request that can no longer accept a response."""
        if "interaction.closed" not in self._capabilities:
            raise ProtocolError("ADAPTER_METHOD_UNSUPPORTED")
        await self._emit("interaction.closed", {"roundKey": self.round_key,
                                                 "interactionKey": interaction_key,
                                                 "reason": reason})
        pending = self._questions.get(interaction_key)
        if pending is not None and not pending[1].done():
            pending[1].cancel()

    def _register_tool(self, request):
        if "tool.request" not in self._capabilities:
            raise ProtocolError("ADAPTER_METHOD_UNSUPPORTED")
        if self.cancelled.is_set() or self._closed:
            raise asyncio.CancelledError()
        if self._suspending:
            raise ProtocolError("ADAPTER_ROUND_SUSPENDING")
        if not self._started or not isinstance(request, ToolCallRequest):
            raise ProtocolError()
        payload = request.wire()
        previous = self._tool_requests.get(request.local_tool_call_key)
        if previous is not None:
            if previous[0] != payload:
                raise ProtocolError("ADAPTER_IDEMPOTENCY_CONFLICT")
            return payload, previous[1], False
        if len(self._tool_requests) >= 256 or sum(
                not future.done() for _, future in self._tool_requests.values()) >= 8:
            raise ProtocolError("ADAPTER_TOOL_BUSY")
        future = asyncio.get_running_loop().create_future()
        self._tool_requests[request.local_tool_call_key] = (payload, future)
        return payload, future, True

    async def request_tool(self, request: ToolCallRequest) -> ToolResult:
        """Publish one keyed tool call and wait for the platform-owned result."""
        payload, future, created = self._register_tool(request)
        if created:
            try:
                await self._emit("tool.request", {"roundKey": self.round_key, **payload})
            except BaseException:
                future.cancel()
                raise
        return await asyncio.shield(future)

    async def wait_for_tool_result(self, local_tool_call_key: str) -> ToolResult:
        """Wait for an already-published tool request without publishing again."""
        pending = self._tool_requests.get(local_tool_call_key)
        if pending is None:
            raise ProtocolError("ADAPTER_TOOL_UNKNOWN")
        return await asyncio.shield(pending[1])

    async def tool_received(self, result: ToolResult, *, status=ToolDeliveryStatus.RECEIVED):
        """Report native receipt after the downstream Agent accepted the result."""
        if not isinstance(result, ToolResult) or not isinstance(status, ToolDeliveryStatus):
            raise ProtocolError()
        saved = self._tool_results.get(result.local_tool_call_key)
        if saved != result:
            raise ProtocolError("ADAPTER_TOOL_UNKNOWN")
        payload = {"roundKey": self.round_key, "localToolCallKey": result.local_tool_call_key,
                   "operationId": result.operation_id, "status": status.value}
        if self._tool_receipt is None:
            await self._emit("tool.received", payload)
        else:
            await self._tool_receipt(payload)

    async def close_tool(self, local_tool_call_key: str, *, reason: str):
        pending = self._tool_requests.get(local_tool_call_key)
        if pending is None:
            return
        await self._emit("tool.closed", {"roundKey": self.round_key,
                                         "localToolCallKey": local_tool_call_key,
                                         "reason": reason})
        if not pending[1].done():
            pending[1].cancel()


Handler = Callable[[RunRequest, ExecutionContext], Awaitable[RoundResult]]


class AgentAdapter:
    def __init__(self, *, name: str, version: str, capabilities=()):
        if not name or not version:
            raise ValueError("name and version are required")
        self.name, self.version = name, version
        self._handlers = {}
        if not isinstance(capabilities, (tuple, list, set, frozenset)) or any(
                value not in {"interaction.request", "tool.request"} for value in capabilities):
            raise ValueError("unsupported optional capability")
        self._optional_capabilities = frozenset(capabilities)

    def _register(self, kind, handler):
        if kind in self._handlers or not inspect.iscoroutinefunction(handler):
            raise ValueError("handler must be a unique async function")
        parameters = tuple(inspect.signature(handler).parameters.values())
        if len(parameters) != 2 or any(p.kind not in (p.POSITIONAL_ONLY, p.POSITIONAL_OR_KEYWORD) for p in parameters):
            raise ValueError("handler requires request and context")
        self._handlers[kind] = handler
        return handler

    def run(self, handler: Handler):
        return self._register("execution.start", handler)

    def resume(self, handler: Handler):
        return self._register("execution.resume", handler)

    def message(self, handler: Callable[[UserMessage, ExecutionContext], Awaitable[DeliveryStatus]]):
        return self._register("user.message", handler)

    def suspend(self, handler: Callable[[RunRequest, ExecutionContext], Awaitable[str]]):
        """Handler quiesces native execution and returns its real session reference."""
        return self._register("execution.suspend", handler)

    @property
    def capabilities(self):
        values = {"activity", "execution.interrupt", *self._handlers.keys()}
        if "interaction.request" in self._optional_capabilities:
            values.update(("interaction.request", "interaction.respond", "interaction.received", "interaction.closed"))
        if "tool.request" in self._optional_capabilities:
            values.update(("tool.request", "tool.result", "tool.received", "tool.closed"))
        if "user.message" in values:
            values.add("message.received")
        if "execution.suspend" in values:
            if "execution.resume" not in values:
                raise ValueError("suspend requires a resume handler")
            values.add("execution.suspended")
        return sorted(values)

    async def serve_streams(self, reader, writer):
        """One initialized session. Caller owns stream creation and closure."""
        if "execution.start" not in self._handlers:
            raise ValueError("run handler is required")
        capabilities = frozenset(self.capabilities)
        write_lock = asyncio.Lock()
        active = None
        current = None
        ready = False
        sequence = 0
        # Bounded session history. Refuse new rounds rather than evicting keys
        # and accidentally treating an old duplicate as a new execution.
        rounds = OrderedDict()
        request_keys = {}
        deliveries = {}
        delivery_payloads = {}
        controls = set()

        def spawn(coroutine, context):
            task = asyncio.create_task(coroutine)
            controls.add(task)
            context._controls.add(task)
            task.add_done_callback(controls.discard)
            task.add_done_callback(context._controls.discard)
            # Retrieve transport failures even if EOF follows a completed control.
            def observed(done):
                if ((done.cancelled() and not context.cancelled.is_set())
                        or (not done.cancelled() and done.exception() is not None)):
                    context._control_unconfirmed = True
            task.add_done_callback(observed)
            return task

        async def drain_controls(context):
            pending = {task for task in context._controls if task is not asyncio.current_task()}
            for task in pending:
                if not task.done() and not task.cancelling():
                    task.cancel()
            if pending:
                done, remaining = await asyncio.wait(pending, timeout=5)
                if (remaining or any(
                        (task.cancelled() and not context.cancelled.is_set())
                        or (not task.cancelled() and task.exception() is not None)
                        for task in done)):
                    context._control_unconfirmed = True
            if context._control_unconfirmed:
                raise ProtocolError("ADAPTER_CONTROL_RECEIPT_UNCONFIRMED")

        async def emit(method, payload, reply_to=None):
            nonlocal sequence
            async with write_lock:
                sequence += 1
                frame = dict(protocol=VERSION, method=method, key=f"adapter:{sequence}", payload=payload)
                if reply_to is not None:
                    frame["replyTo"] = reply_to
                raw = encode(frame)
                decode(raw, direction="up")
                writer.write(raw)
                await asyncio.wait_for(writer.drain(), timeout=5)

        async def finalize_interrupted_deliveries(context):
            """Make every accepted control replayable before cancelling handlers."""
            for delivery_key, record in tuple(deliveries.items()):
                if delivery_key[0] != context.round_key or record[1] is not None:
                    continue
                method = delivery_key[1]
                payload = delivery_payloads.get(delivery_key, {})
                if method == "user.message":
                    result_method = "message.received"
                    result = {"roundKey": context.round_key,
                              "messageKey": payload.get("messageKey", delivery_key[2]),
                              "status": "UNKNOWN"}
                elif method == "execution.suspend":
                    result_method, result = "error", {"code": "ADAPTER_SUSPEND_FAILED"}
                    context._suspending = False
                elif method == "tool.result":
                    result_method = "tool.received"
                    result = {"roundKey": context.round_key,
                              "localToolCallKey": payload.get("localToolCallKey", delivery_key[2]),
                              "operationId": payload.get("operationId", ""),
                              "status": "UNKNOWN"}
                else:
                    continue
                record[1] = (result_method, result)
                replies, record[2] = record[2], []
                for reply in replies:
                    await emit(result_method, result, reply)

        async def execute(frame, request, context):
            try:
                result = await self._handlers[frame["method"]](request, context)
                if not isinstance(result, RoundResult) or not isinstance(result.outcome, RoundOutcome):
                    raise ProtocolError()
                if not context._started:
                    raise ProtocolError("ADAPTER_NATIVE_NOT_STARTED")
            except asyncio.CancelledError:
                result = RoundResult(RoundOutcome.CANCELLED)
            except Exception:
                # Exceptions may include paths, credentials or native bodies.
                # A handler that failed before reporting native startup cannot
                # emit execution.finished: the Supervisor intentionally
                # requires a preceding execution.started for a new round.
                if not context._started:
                    context._close()
                    await emit("error", {"code": "ADAPTER_EXECUTION_FAILED"}, frame["key"])
                    return
                result = RoundResult(RoundOutcome.FAILED, "ADAPTER_EXECUTION_FAILED")
            context._close()
            if context._suspended:
                return
            try:
                await drain_controls(context)
            except ProtocolError as error:
                await emit("error", {"code": error.code}, frame["key"])
                return
            payload = {"roundKey": request.round_key, "outcome": result.outcome.value,
                       "summary": result.summary, "sessionReference": result.session_reference}
            rounds[request.round_key][1] = payload
            await emit("execution.finished", payload, frame["key"])

        async def deliver(frame, context, execution, record):
            method, payload = frame["method"], frame["payload"]
            result_method = "message.received" if method == "user.message" else "execution.suspended"
            try:
                if method == "user.message":
                    status = await self._handlers[method](UserMessage(payload["roundKey"], payload["messageKey"], payload["text"]), context)
                    if not isinstance(status, DeliveryStatus):
                        raise ProtocolError()
                    result = {"roundKey": context.round_key, "messageKey": payload["messageKey"], "status": status.value}
                else:
                    reference = await self._handlers[method](RunRequest(context.round_key, {}), context)
                    interaction_text(reference, 4096, nonempty=True)
                    if execution.done() or context._closed or context.cancelled.is_set():
                        raise ProtocolError()
                    context._suspended = True
                    context._close()
                    execution.cancel()
                    await execution
                    await drain_controls(context)
                    result = {"roundKey": context.round_key, "sessionReference": reference}
                    rounds[context.round_key][1:] = [result, "execution.suspended"]
            except (Exception, asyncio.CancelledError) as error:
                if method == "user.message":
                    result = {"roundKey": context.round_key, "messageKey": payload["messageKey"], "status": "UNKNOWN"}
                else:
                    code = "ADAPTER_CONTROL_RECEIPT_UNCONFIRMED" if isinstance(error, ProtocolError) and error.code == "ADAPTER_CONTROL_RECEIPT_UNCONFIRMED" else "ADAPTER_SUSPEND_FAILED"
                    result_method, result = "error", {"code": code}
                    context._suspending = False
            if record[1] is not None:
                return
            record[1] = (result_method, result)
            replies, record[2] = record[2], []
            for reply in replies:
                await emit(result_method, result, reply)

        try:
            while True:
                try:
                    raw = await reader.readline()
                    if not raw:
                        break
                    frame = decode(raw, direction="down")
                except ProtocolError as error:
                    await emit("error", {"code": error.code})
                    # An unsupported method cannot execute anything. Keep the
                    # connection usable for cancellation or a supported call.
                    if error.code == "ADAPTER_METHOD_UNSUPPORTED":
                        continue
                    break
                except ValueError:
                    await emit("error", {"code": "ADAPTER_PROTOCOL_INVALID"})
                    break
                method, payload = frame["method"], frame["payload"]
                request_digest = hashlib.sha256(encode({"method": method, "payload": payload})).digest()
                previous_request = request_keys.get(frame["key"])
                if previous_request is not None and previous_request != request_digest:
                    await emit("error", {"code": "ADAPTER_IDEMPOTENCY_CONFLICT"}, frame["key"])
                    continue
                if previous_request is None:
                    if len(request_keys) >= 2048:
                        await emit("error", {"code": "ADAPTER_SESSION_LIMIT"}, frame["key"])
                        continue
                    request_keys[frame["key"]] = request_digest
                if method == "initialize":
                    if ready:
                        await emit("error", {"code": "ADAPTER_ALREADY_INITIALIZED"}, frame["key"])
                        continue
                    ready = True
                    await emit("ready", {"name": self.name, "version": self.version,
                                         "capabilities": self.capabilities}, frame["key"])
                    continue
                if not ready:
                    await emit("error", {"code": "ADAPTER_NOT_INITIALIZED"}, frame["key"])
                    continue
                round_key = payload.get("roundKey")
                if not isinstance(round_key, str) or not round_key or len(round_key) > 128:
                    await emit("error", {"code": "ADAPTER_ROUND_INVALID"}, frame["key"])
                    continue
                if method == "execution.interrupt":
                    if current is None or current.round_key != round_key or active.done() or current._closed:
                        await emit("error", {"code": "ADAPTER_ROUND_NOT_RUNNING"}, frame["key"])
                        continue
                    current.cancelled.set()
                    await finalize_interrupted_deliveries(current)
                    active.cancel()
                    for task in tuple(controls):
                        task.cancel()
                    await emit("control.ack", {"roundKey": round_key}, frame["key"])
                    continue
                if method in {"interaction.respond", "user.message", "execution.suspend", "tool.result"}:
                    if method not in capabilities:
                        await emit("error", {"code": "ADAPTER_METHOD_UNSUPPORTED"}, frame["key"])
                        continue
                    delivery_key = (round_key, method, payload.get(
                        "interactionKey", payload.get("messageKey", payload.get("localToolCallKey", ""))))
                    record = deliveries.get(delivery_key)
                    if record is not None:
                        if record[0] != request_digest:
                            await emit("error", {"code": "ADAPTER_IDEMPOTENCY_CONFLICT"}, frame["key"])
                        elif record[1] is not None:
                            await emit(*record[1], frame["key"])
                        elif frame["key"] not in record[2]:
                            record[2].append(frame["key"])
                        continue
                    if len(deliveries) >= 2048:
                        await emit("error", {"code": "ADAPTER_SESSION_LIMIT"}, frame["key"])
                        continue
                    if current is None or current.round_key != round_key or active.done() or current._closed or current.cancelled.is_set():
                        await emit("error", {"code": "ADAPTER_ROUND_NOT_RUNNING"}, frame["key"])
                        continue
                    if current._suspending:
                        await emit("error", {"code": "ADAPTER_ROUND_SUSPENDING"}, frame["key"])
                        continue
                    if method == "interaction.respond":
                        question = current._questions.get(payload["interactionKey"])
                        if question is None or question[1].done():
                            await emit("error", {"code": "ADAPTER_INTERACTION_UNKNOWN"}, frame["key"])
                            continue
                        answers = answers_from_wire(payload["answers"])
                        try:
                            validate_answers(question[0], answers)
                        except ProtocolError as error:
                            await emit("error", {"code": error.code}, frame["key"])
                            continue
                        expected = current._restored_responses.get(payload["interactionKey"])
                        response = InteractionResponse(payload["interactionKey"], payload["responseId"], answers)
                        if expected is not None and expected != encode(response.wire()):
                            await emit("error", {"code": "ADAPTER_IDEMPOTENCY_CONFLICT"}, frame["key"])
                            continue
                        result = {"roundKey": round_key, "interactionKey": payload["interactionKey"],
                                  "responseId": payload["responseId"], "status": "RECEIVED"}
                        deliveries[delivery_key] = [request_digest, ("interaction.received", result), []]
                        current._responses[payload["interactionKey"]] = response
                        # Accept atomically before awaiting transport: an ask
                        # timeout during drain must not discard an accepted answer.
                        question[1].set_result(answers)
                        await emit("interaction.received", result, frame["key"])
                    elif method == "tool.result":
                        pending = current._tool_requests.get(payload["localToolCallKey"])
                        if pending is None or pending[1].done():
                            await emit("error", {"code": "ADAPTER_TOOL_UNKNOWN"}, frame["key"])
                            continue
                        result = ToolResult.from_wire({key: payload[key] for key in (
                            "nativeToolCallId", "localToolCallKey", "operationId", "status",
                            "exitCode", "summary", "output", "resultDigest", "reasonCode")})
                        if pending[0]["nativeToolCallId"] != result.native_tool_call_id:
                            await emit("error", {"code": "ADAPTER_TOOL_IDENTITY_CONFLICT"}, frame["key"])
                            continue
                        deliveries[delivery_key] = [request_digest, None, [frame["key"]]]
                        delivery_payloads[delivery_key] = payload
                        current._tool_results[result.local_tool_call_key] = result
                        pending[1].set_result(result)
                    else:
                        record = [request_digest, None, [frame["key"]]]
                        deliveries[delivery_key] = record
                        delivery_payloads[delivery_key] = payload
                        if method == "execution.suspend":
                            # Fence new input before the native handler snapshots
                            # its session; accepted answers cannot arrive afterward.
                            current._suspending = True
                        spawn(deliver(frame, current, active, record), current)
                        # Enter the handler's cancellation guard before processing
                        # an already-buffered interrupt/EOF. Otherwise cancelling
                        # an unstarted Task leaves its delivery pending forever.
                        await asyncio.sleep(0)
                    continue
                identity = request_digest
                if round_key in rounds:
                    previous, result, _ = rounds[round_key]
                    if previous != identity:
                        await emit("error", {"code": "ADAPTER_IDEMPOTENCY_CONFLICT"}, frame["key"])
                    elif result is not None:
                        await emit(rounds[round_key][2], result, frame["key"])
                    else:
                        await emit("control.ack", {"roundKey": round_key}, frame["key"])
                    continue
                if method not in self._handlers:
                    await emit("error", {"code": "ADAPTER_METHOD_UNSUPPORTED"}, frame["key"])
                    continue
                if current is not None and current._control_unconfirmed:
                    await emit("error", {"code": "ADAPTER_CONTROL_RECEIPT_UNCONFIRMED"}, frame["key"])
                    continue
                if active is not None and not active.done() and rounds[current.round_key][1] is None:
                    await emit("error", {"code": "ADAPTER_BUSY"}, frame["key"])
                    continue
                if len(rounds) >= 256:
                    await emit("error", {"code": "ADAPTER_SESSION_LIMIT"}, frame["key"])
                    continue
                inputs = payload.get("inputs")
                if not isinstance(inputs, dict):
                    await emit("error", {"code": "ADAPTER_INPUT_INVALID"}, frame["key"])
                    continue
                if method == "execution.resume":
                    reference = payload.get("sessionReference")
                    if not isinstance(reference, str) or not reference or len(reference) > 4096:
                        await emit("error", {"code": "ADAPTER_INPUT_INVALID"}, frame["key"])
                        continue
                    request = ResumeRequest(round_key, inputs, reference)
                else:
                    request = RunRequest(round_key, inputs)
                rounds[round_key] = [identity, None, "execution.finished"]
                async def round_emit(method, payload, start_key=frame["key"]):
                    await emit(method, payload, start_key)
                current = ExecutionContext(round_key, round_emit, capabilities=capabilities)
                async def tool_receipt(payload, active_context=current):
                    delivery_key = (active_context.round_key, "tool.result", payload["localToolCallKey"])
                    record = deliveries.get(delivery_key)
                    if record is None:
                        raise ProtocolError("ADAPTER_TOOL_UNKNOWN")
                    result = ("tool.received", payload)
                    if record[1] is not None and record[1] != result:
                        raise ProtocolError("ADAPTER_IDEMPOTENCY_CONFLICT")
                    record[1] = result
                    replies, record[2] = record[2], []
                    for reply in replies:
                        await emit("tool.received", payload, reply)
                current._tool_receipt = tool_receipt
                active = asyncio.create_task(execute(frame, request, current))
                # Enter the execution handler before consuming buffered controls.
                await asyncio.sleep(0)
        finally:
            pending_controls = tuple(controls)
            for task in pending_controls:
                task.cancel()
            if pending_controls:
                await asyncio.wait_for(asyncio.gather(*pending_controls, return_exceptions=True), timeout=5)
            if active is not None:
                if not active.done():
                    current.cancelled.set()
                    active.cancel()
                try:
                    await asyncio.wait_for(active, timeout=5)
                except (asyncio.CancelledError, asyncio.TimeoutError, ConnectionError, ProtocolError):
                    pass

    def serve(self):
        async def main():
            loop = asyncio.get_running_loop()
            reader = asyncio.StreamReader(limit=MAX_FRAME_BYTES)
            await loop.connect_read_pipe(lambda: asyncio.StreamReaderProtocol(reader), sys.stdin.buffer)
            transport, protocol = await loop.connect_write_pipe(asyncio.streams.FlowControlMixin, sys.stdout.buffer)
            writer = asyncio.StreamWriter(transport, protocol, None, loop)
            try:
                await self.serve_streams(reader, writer)
            finally:
                writer.close()
        asyncio.run(main())
