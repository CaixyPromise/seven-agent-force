"""Deterministic network-tool Adapter fixture; no provider or network access."""
from __future__ import annotations

import hashlib
import json
from pathlib import Path
import sys

# The admitted launcher uses python -I. Import only from protected image paths;
# do not let the Task repository or environment shadow the runtime SDK.
sys.path[:0] = ["/opt/seven-worker/result-runtime", "/opt/seven/adapter"]

from seven_agent.force.agent import (AgentAdapter, RoundResult, ToolCallRequest,
                                            ToolDeliveryStatus, ToolResult, ToolTarget)

FIXTURE = Path(__file__).absolute().parent / "fixtures" / "tool.json"
FIXTURE_DIGEST = "a95c16f1e3768e3718cc841f280e8a5e7f437db9ef3ff75a605480567d699c0d"
SESSION_REFERENCE = "fake-tool-session-" + FIXTURE_DIGEST[:32]


def load_fixture():
    raw = FIXTURE.read_bytes()
    if hashlib.sha256(raw).hexdigest() != FIXTURE_DIGEST:
        raise ValueError("FAKE_TOOL_FIXTURE_INVALID")
    value = json.loads(raw)
    if not isinstance(value, dict) or set(value) != {
            "role", "scenario", "nativeToolCallId", "localToolCallKey", "purpose",
            "command", "cwd", "targets"}:
        raise ValueError("FAKE_TOOL_FIXTURE_INVALID")
    if value["role"] != "AGENT" or value["scenario"] != "NETWORK_TOOL_CALL":
        raise ValueError("FAKE_TOOL_FIXTURE_INVALID")
    value["targets"] = [ToolTarget.from_wire(target).wire() for target in value["targets"]]
    return value


adapter = AgentAdapter(name="fake", version="batch4-tool", capabilities=("tool.request",))


@adapter.run
async def run(request, context):
    fixture = load_fixture()
    await context.started()
    result = await context.request_tool(ToolCallRequest(
        fixture["nativeToolCallId"], fixture["localToolCallKey"],
        "run_command_with_network_access", {
            "purpose": fixture["purpose"], "command": fixture["command"],
            "cwd": fixture["cwd"], "targets": fixture["targets"],
        }))
    await context.tool_received(result, status=ToolDeliveryStatus.RECEIVED)
    await context.emit_message("The deterministic network operation result was returned to the Fake Adapter.")
    return RoundResult.finished(summary="Deterministic network operation consumed",
                                session_reference=SESSION_REFERENCE)


@adapter.resume
async def resume(request, context):
    if request.session_reference != SESSION_REFERENCE or not isinstance(request.inputs, dict):
        raise ValueError("FAKE_OPERATION_RESTORE_INVALID")
    if set(request.inputs) != {"operationToolResult"}:
        raise ValueError("FAKE_OPERATION_RESTORE_INVALID")
    try:
        result = ToolResult.from_wire(request.inputs["operationToolResult"])
    except Exception:
        raise ValueError("FAKE_OPERATION_RESTORE_INVALID") from None
    fixture = load_fixture()
    if (result.native_tool_call_id != fixture["nativeToolCallId"]
            or result.local_tool_call_key != fixture["localToolCallKey"]):
        raise ValueError("FAKE_OPERATION_RESTORE_INVALID")
    await context.started()
    await context.emit_message("The restored network operation result was consumed without issuing another tool request.")
    return RoundResult.finished(summary="Restored network operation consumed",
                                session_reference=SESSION_REFERENCE)


if __name__ == "__main__":
    adapter.serve()
