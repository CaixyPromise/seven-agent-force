import asyncio
import hashlib
import unittest

from seven_agent.force._protocol import ProtocolError, VERSION, decode, encode
from seven_agent.force.agent import (AgentAdapter, RoundResult, ToolCallRequest,
                                            ToolDefinition, ToolDeliveryStatus, ToolResult,
                                            ToolTarget)


class Writer:
    def __init__(self):
        self.frames = []

    def write(self, raw):
        self.frames.append(decode(raw, direction="up"))

    async def drain(self):
        await asyncio.sleep(0)


class ToolProtocolTests(unittest.IsolatedAsyncioTestCase):
    def request(self):
        target = ToolTarget("registry.npmjs.org", "https", 443)
        return ToolCallRequest("codex-call-1", "native:call-1",
                               "run_command_with_network_access", {
            "purpose": "Install locked dependencies",
            "command": "pnpm install --frozen-lockfile",
            "cwd": ".",
            "targets": [target.wire()],
        })

    def result(self):
        return ToolResult("codex-call-1", "native:call-1", "101", "SUCCEEDED",
                          0, "Installed", "ok",
                          "a" * 64, "")

    def git_delivery_request(self, summary="  Pagination fixed.  "):
        return ToolCallRequest("codex-git-call-1", "native:git:1",
                               "request_git_delivery", {"summary": summary})

    def test_git_delivery_request_is_summary_only_and_utf8_byte_bounded(self):
        wire = self.git_delivery_request().wire()
        self.assertEqual(wire["arguments"], {"summary": "Pagination fixed."})
        self.assertEqual(wire["argumentsDigest"],
                         hashlib.sha256(b'{"summary":"Pagination fixed."}').hexdigest())
        for arguments in ({"summary": ""}, {"summary": "   "},
                          {"summary": "x", "remote": "https://example.com"},
                          {"summary": "界" * 683}):
            with self.subTest(arguments=arguments), self.assertRaises(ProtocolError):
                ToolCallRequest("codex-git-call-1", "native:git:1",
                                "request_git_delivery", arguments).wire()
        scheduled = ToolResult("codex-git-call-1", "native:git:1", "101", "SCHEDULED",
                               None, "Git delivery intent saved.", "", "a" * 64, "")
        self.assertEqual(ToolResult.from_wire(scheduled.wire()), scheduled)
        with self.assertRaises(ProtocolError):
            ToolResult("codex-git-call-1", "native:git:1", "101", "SCHEDULED",
                       0, "Git delivery intent saved.", "", "a" * 64, "").wire()

    def test_typed_objects_validate_exact_network_contract_parts(self):
        definition = ToolDefinition("run_command_with_network_access", "Run with access", {
            "type": "object", "additionalProperties": False,
            "properties": {"command": {"type": "string"}}, "required": ["command"],
        })
        self.assertEqual(definition.wire()["type"], "function")
        self.assertEqual(self.request().wire()["argumentsDigest"],
                         "ff1085ef490cee08d49e18922b0f5f3390e20b8643d837c83cec15517cc62765")
        self.assertEqual(ToolResult.from_wire(self.result().wire()), self.result())
        self.assertEqual(
            ToolTarget.from_wire({"host": "Registry.NPMJS.org.", "protocol": " HTTPS ", "port": 443}).wire(),
            {"host": "registry.npmjs.org", "protocol": "https", "port": 443},
        )
        normalized = ToolCallRequest("codex-normalized", "native:normalized",
                                     "run_command_with_network_access", {
            "purpose": " install ", "command": " pnpm install ", "cwd": " . ",
            "targets": [
                {"host": "B.example.com", "protocol": "HTTPS", "port": 443},
                {"host": "a.example.com.", "protocol": " https ", "port": 443},
            ],
        }).wire()
        self.assertEqual(normalized["arguments"], {
            "purpose": "install", "command": "pnpm install", "cwd": ".",
            "targets": [
                {"host": "a.example.com", "protocol": "https", "port": 443},
                {"host": "b.example.com", "protocol": "https", "port": 443},
            ],
        })
        for target in (("127.0.0.1", "https", 443), ("localhost", "https", 443),
                       ("*.example.com", "https", 443),
                       ("registry.npmjs.org", "tcp", 443), ("registry.npmjs.org", "https", 0)):
            with self.subTest(target=target), self.assertRaises(ProtocolError):
                ToolTarget(*target).wire()
        with self.assertRaises(ProtocolError):
            ToolCallRequest("codex-multi-port", "native:multi-port",
                            "run_command_with_network_access", {
                "purpose": "install", "command": "echo x", "cwd": ".",
                "targets": [
                    {"host": "example.com", "protocol": "http", "port": 80},
                    {"host": "example.com", "protocol": "https", "port": 443},
                ],
            }).wire()

    def test_strict_wire_rejects_digest_extra_and_invalid_result(self):
        payload = {"roundKey": "r", **self.request().wire()}
        frame = {"protocol": VERSION, "method": "tool.request", "key": "request", "payload": payload}
        self.assertEqual(decode(encode(frame), direction="up"), frame)
        for change in ({"argumentsDigest": "0" * 64}, {"extra": True}):
            broken = dict(payload, **change)
            with self.assertRaises(ProtocolError):
                decode(encode(dict(frame, payload=broken)), direction="up")
        with self.assertRaises(ProtocolError):
            ToolResult("codex-call-1", "native:call-1", "101", "SUCCEEDED", None).wire()
        for native_id in ("", "bad\ncall", "\ud800", "x" * 513):
            with self.subTest(native_id=repr(native_id)), self.assertRaises(ProtocolError):
                ToolCallRequest(native_id, "native:call-1",
                                "run_command_with_network_access",
                                self.request().arguments).wire()

    async def test_request_wait_result_and_native_receipt_are_distinct(self):
        agent = AgentAdapter(name="tool-fixture", version="1", capabilities=("tool.request",))
        continued = asyncio.Event()

        @agent.run
        async def run(request, context):
            await context.started()
            result = await context.request_tool(self.request())
            self.assertEqual(await context.wait_for_tool_result("native:call-1"), result)
            await context.tool_received(result, status=ToolDeliveryStatus.RECEIVED)
            continued.set()
            return RoundResult.finished()

        reader, writer = asyncio.StreamReader(), Writer()
        session = asyncio.create_task(agent.serve_streams(reader, writer))

        def send(method, key, **payload):
            reader.feed_data(encode({"protocol": VERSION, "method": method, "key": key, "payload": payload}))

        send("initialize", "init")
        send("execution.start", "start", roundKey="r", inputs={})
        for _ in range(100):
            if any(frame["method"] == "tool.request" for frame in writer.frames):
                break
            await asyncio.sleep(.01)
        self.assertFalse(continued.is_set())
        send("tool.result", "result", roundKey="r", **self.result().wire())
        await asyncio.wait_for(continued.wait(), 1)
        receipts = [frame for frame in writer.frames if frame["method"] == "tool.received"]
        self.assertEqual(len(receipts), 1)
        self.assertEqual(receipts[0]["replyTo"], "result")
        send("tool.result", "result-retry", roundKey="r", **self.result().wire())
        for _ in range(20):
            await asyncio.sleep(0)
        receipts = [frame for frame in writer.frames if frame["method"] == "tool.received"]
        self.assertEqual(len(receipts), 2)
        self.assertEqual(receipts[-1]["replyTo"], "result-retry")
        reader.feed_eof()
        await asyncio.wait_for(session, 1)

    async def test_result_native_call_identity_must_match_pending_request(self):
        agent = AgentAdapter(name="tool-fixture", version="1", capabilities=("tool.request",))

        @agent.run
        async def run(request, context):
            await context.started()
            await context.request_tool(self.request())
            return RoundResult.finished()

        reader, writer = asyncio.StreamReader(), Writer()
        session = asyncio.create_task(agent.serve_streams(reader, writer))

        def send(method, key, **payload):
            reader.feed_data(encode({"protocol": VERSION, "method": method,
                                     "key": key, "payload": payload}))

        send("initialize", "init")
        send("execution.start", "start", roundKey="r", inputs={})
        for _ in range(100):
            if any(frame["method"] == "tool.request" for frame in writer.frames):
                break
            await asyncio.sleep(.01)
        wrong = {**self.result().wire(), "nativeToolCallId": "different-native-call"}
        send("tool.result", "wrong-result", roundKey="r", **wrong)
        for _ in range(100):
            if any(frame.get("replyTo") == "wrong-result" for frame in writer.frames):
                break
            await asyncio.sleep(.01)
        error = next(frame for frame in writer.frames
                     if frame.get("replyTo") == "wrong-result")
        self.assertEqual(error["payload"]["code"], "ADAPTER_TOOL_IDENTITY_CONFLICT")
        send("tool.result", "result", roundKey="r", **self.result().wire())
        for _ in range(100):
            if any(frame["method"] == "execution.finished" for frame in writer.frames):
                break
            await asyncio.sleep(.01)
        reader.feed_eof()
        await asyncio.wait_for(session, 1)

    async def test_disabled_capability_never_publishes(self):
        events = []
        from seven_agent.force.agent import ExecutionContext
        async def emit(method, payload):
            events.append((method, payload))
        context = ExecutionContext("r", emit)
        await context.started()
        with self.assertRaises(ProtocolError) as caught:
            await context.request_tool(self.request())
        self.assertEqual(caught.exception.code, "ADAPTER_METHOD_UNSUPPORTED")
        self.assertFalse(any(method == "tool.request" for method, _ in events))


if __name__ == "__main__":
    unittest.main()
