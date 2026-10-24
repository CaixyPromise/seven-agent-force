import asyncio
import hashlib
from pathlib import Path
import sys
import unittest

ROOT = Path(__file__).resolve().parents[1]
sys.path[:0] = [str(ROOT), str(ROOT.parents[1] / "runtime-sdk" / "src")]

import git_delivery_tool
from seven_agent.force.agent import ExecutionContext, RunRequest, ToolResult


class FakeGitDeliveryToolTests(unittest.IsolatedAsyncioTestCase):
    async def test_fixed_git_delivery_request_uses_summary_only(self):
        events = []

        async def emit(method, payload):
            events.append((method, payload))

        context = ExecutionContext("round-1", emit,
                                   capabilities=git_delivery_tool.adapter.capabilities)
        execution = asyncio.create_task(git_delivery_tool.run(
            RunRequest("round-1", {}), context))
        for _ in range(100):
            requests = [payload for method, payload in events if method == "tool.request"]
            if requests:
                break
            await asyncio.sleep(.01)
        request = requests[0]
        self.assertEqual(request["toolName"], "request_git_delivery")
        self.assertEqual(set(request["arguments"]), {"summary"})
        pending = context._tool_requests[request["localToolCallKey"]][1]
        result = ToolResult(request["nativeToolCallId"], request["localToolCallKey"],
                            "101", "SCHEDULED", None,
                            "Git delivery intent saved and scheduled after required gates.",
                            "", "a" * 64, "")
        context._tool_results[request["localToolCallKey"]] = result
        pending.set_result(result)
        completed = await asyncio.wait_for(execution, 1)
        self.assertEqual(completed.summary, "Deterministic Git delivery intent scheduled")

    def test_fixture_digest_is_fixed(self):
        self.assertEqual(hashlib.sha256(git_delivery_tool.FIXTURE.read_bytes()).hexdigest(),
                         git_delivery_tool.FIXTURE_DIGEST)
        self.assertEqual(git_delivery_tool.load_fixture()["scenario"],
                         "GIT_DELIVERY_TOOL_CALL")


if __name__ == "__main__":
    unittest.main()
