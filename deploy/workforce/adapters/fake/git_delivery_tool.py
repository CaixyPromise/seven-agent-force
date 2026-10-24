"""Deterministic Git-delivery intent fixture; it never accesses Git."""
from __future__ import annotations

import hashlib
import json
from pathlib import Path
import sys

sys.path[:0] = ["/opt/seven-worker/result-runtime", "/opt/seven/adapter"]

from seven_agent.force.agent import AgentAdapter, RoundResult, ToolCallRequest

FIXTURE = Path(__file__).absolute().parent / "fixtures" / "git-delivery-tool.json"
FIXTURE_DIGEST = "9b45ff30fb35fab5c04decca1acbbbb6240803e03e307205b72cf1d9baff1270"
SESSION_REFERENCE = "fake-git-delivery-session-" + FIXTURE_DIGEST[:32]


def load_fixture():
    raw = FIXTURE.read_bytes()
    if hashlib.sha256(raw).hexdigest() != FIXTURE_DIGEST:
        raise ValueError("FAKE_GIT_DELIVERY_FIXTURE_INVALID")
    value = json.loads(raw)
    if (not isinstance(value, dict) or set(value) != {
            "role", "scenario", "nativeToolCallId", "localToolCallKey", "summary"}
            or value["role"] != "AGENT"
            or value["scenario"] != "GIT_DELIVERY_TOOL_CALL"):
        raise ValueError("FAKE_GIT_DELIVERY_FIXTURE_INVALID")
    return value


adapter = AgentAdapter(name="fake", version="batch5-git-delivery-tool",
                       capabilities=("tool.request",))


@adapter.run
async def run(request, context):
    fixture = load_fixture()
    await context.started()
    await context.request_tool(ToolCallRequest(
        fixture["nativeToolCallId"], fixture["localToolCallKey"],
        "request_git_delivery", {"summary": fixture["summary"]}))
    return RoundResult.finished(summary="Deterministic Git delivery intent scheduled",
                                session_reference=SESSION_REFERENCE)


if __name__ == "__main__":
    adapter.serve()
