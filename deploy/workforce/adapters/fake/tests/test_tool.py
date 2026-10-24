import asyncio
import hashlib
from pathlib import Path
import sys
import unittest

ROOT = Path(__file__).resolve().parents[1]
sys.path[:0] = [str(ROOT), str(ROOT.parents[1] / "runtime-sdk" / "src")]

import tool
from seven_agent.force.agent import ExecutionContext, ResumeRequest, RunRequest, ToolResult


class FakeToolTests(unittest.IsolatedAsyncioTestCase):
    async def test_fixed_tool_request_consumes_one_platform_result(self):
        events = []

        async def emit(method, payload):
            events.append((method, payload))

        context = ExecutionContext("round-1", emit, capabilities=tool.adapter.capabilities)
        execution = asyncio.create_task(tool.run(RunRequest("round-1", {}), context))
        for _ in range(100):
            requests = [payload for method, payload in events if method == "tool.request"]
            if requests:
                break
            await asyncio.sleep(.01)
        request = requests[0]
        self.assertEqual(request["nativeToolCallId"], "fake-native-network-1")
        self.assertEqual(request["toolName"], "run_command_with_network_access")
        self.assertEqual(request["arguments"]["command"], "pnpm install --frozen-lockfile")
        pending = context._tool_requests[request["localToolCallKey"]][1]
        tool_result = ToolResult(request["nativeToolCallId"], request["localToolCallKey"],
                                 "101", "SUCCEEDED", 0,
                                 "Installed", "ok", "a" * 64, "")
        context._tool_results[request["localToolCallKey"]] = tool_result
        pending.set_result(tool_result)
        result = await asyncio.wait_for(execution, 1)
        self.assertEqual(result.summary, "Deterministic network operation consumed")
        self.assertEqual(result.session_reference, tool.SESSION_REFERENCE)
        receipts = [payload for method, payload in events if method == "tool.received"]
        self.assertEqual(receipts, [{"roundKey": "round-1", "localToolCallKey": "fake-network-1",
                                     "operationId": "101", "status": "RECEIVED"}])

    async def test_cold_resume_consumes_saved_result_without_new_request(self):
        events = []

        async def emit(method, payload):
            events.append((method, payload))

        context = ExecutionContext("round-2", emit, capabilities=tool.adapter.capabilities)
        result_value = ToolResult("fake-native-network-1", "fake-network-1",
                                  "101", "SUCCEEDED", 0,
                                  "Installed", "ok", "a" * 64, "")
        result = await tool.resume(ResumeRequest(
            "round-2", {"operationToolResult": result_value.wire()},
            tool.SESSION_REFERENCE), context)

        self.assertEqual(result.summary, "Restored network operation consumed")
        self.assertFalse(any(method == "tool.request" for method, _ in events))
        self.assertFalse(any(method == "tool.received" for method, _ in events))
        self.assertTrue(any(method == "message" and "restored network operation" in payload["text"]
                            for method, payload in events))

    async def test_cold_resume_rejects_stale_native_identity_and_wrong_session(self):
        context = ExecutionContext("round-2", lambda *_: asyncio.sleep(0),
                                   capabilities=tool.adapter.capabilities)
        value = ToolResult("fake-native-network-1", "fake-network-1", "101",
                           "DENIED", None, "", "", "",
                           "USER_DECLINED_NETWORK_ACCESS").wire()
        with self.assertRaisesRegex(ValueError, "FAKE_OPERATION_RESTORE_INVALID"):
            await tool.resume(ResumeRequest(
                "round-2", {"operationToolResult": {**value, "requestId": "json-rpc-id"}},
                tool.SESSION_REFERENCE), context)
        with self.assertRaisesRegex(ValueError, "FAKE_OPERATION_RESTORE_INVALID"):
            await tool.resume(ResumeRequest(
                "round-2", {"operationToolResult": {
                    **value, "nativeToolCallId": "wrong-native-call"}},
                tool.SESSION_REFERENCE), context)
        with self.assertRaisesRegex(ValueError, "FAKE_OPERATION_RESTORE_INVALID"):
            await tool.resume(ResumeRequest(
                "round-2", {"operationToolResult": value}, "wrong-session"), context)

    def test_fixture_digest_is_fixed(self):
        self.assertEqual(hashlib.sha256(tool.FIXTURE.read_bytes()).hexdigest(), tool.FIXTURE_DIGEST)
        self.assertEqual(tool.load_fixture()["scenario"], "NETWORK_TOOL_CALL")


if __name__ == "__main__":
    unittest.main()
