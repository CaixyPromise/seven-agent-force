import asyncio
import unittest

from seven_agent.force._protocol import VERSION, decode, encode
from seven_agent.force.agent import AgentAdapter, RoundResult


class Writer:
    def __init__(self):
        self.frames = []

    def write(self, raw):
        self.frames.append(decode(raw, direction="up"))

    async def drain(self):
        await asyncio.sleep(0)


class AgentTests(unittest.IsolatedAsyncioTestCase):
    async def test_unsupported_method_does_not_execute_and_keeps_handshake_available(self):
        agent = AgentAdapter(name="fake", version="1")

        @agent.run
        async def run(request, context):
            self.fail("unsupported method started an Agent")

        reader, writer = asyncio.StreamReader(), Writer()
        for method in ("unsupported.control", "initialize"):
            reader.feed_data(encode(dict(protocol=VERSION, method=method, key=method, payload={})))
        reader.feed_eof()
        await agent.serve_streams(reader, writer)
        self.assertEqual(writer.frames[0]["payload"]["code"], "ADAPTER_METHOD_UNSUPPORTED")
        self.assertEqual(writer.frames[1]["method"], "ready")

    async def test_duplicate_start_conflict_and_interrupt(self):
        agent = AgentAdapter(name="fake", version="1")
        starts = 0
        started = asyncio.Event()

        @agent.run
        async def run(request, context):
            nonlocal starts
            starts += 1
            await context.started()
            started.set()
            await asyncio.Event().wait()
            return RoundResult.finished()

        reader, writer = asyncio.StreamReader(), Writer()
        session = asyncio.create_task(agent.serve_streams(reader, writer))

        def send(method, key, **payload):
            reader.feed_data(encode(dict(protocol=VERSION, method=method, key=key, payload=payload)))

        send("initialize", "init")
        send("execution.start", "start", roundKey="r1", inputs={})
        await asyncio.wait_for(started.wait(), 1)
        send("execution.start", "duplicate", roundKey="r1", inputs={})
        send("execution.start", "conflict", roundKey="r1", inputs={"different": True})
        send("execution.start", "start", roundKey="r2", inputs={})
        send("execution.interrupt", "cancel", roundKey="r1")
        for _ in range(30):
            await asyncio.sleep(0)
        reader.feed_eof()
        await asyncio.wait_for(session, 1)
        self.assertEqual(starts, 1)
        self.assertTrue(any(f["payload"].get("code") == "ADAPTER_IDEMPOTENCY_CONFLICT" for f in writer.frames))
        self.assertTrue(any(f.get("replyTo") == "start" and f["payload"].get("code") == "ADAPTER_IDEMPOTENCY_CONFLICT" for f in writer.frames))
        final = [f for f in writer.frames if f["method"] == "execution.finished"]
        self.assertEqual(len(final), 1)
        self.assertEqual(final[0]["payload"]["outcome"], "CANCELLED")

    async def test_finished_is_not_platform_success(self):
        result = RoundResult.finished(summary="native round ended")
        self.assertFalse(hasattr(result, "task_status"))

    async def test_handler_failure_before_native_start_is_protocol_error_not_finished(self):
        agent = AgentAdapter(name="fake", version="1")

        @agent.run
        async def run(request, context):
            raise ValueError("untrusted native detail")

        reader, writer = asyncio.StreamReader(), Writer()
        reader.feed_data(encode(dict(protocol=VERSION, method="initialize",
                                     key="init", payload={})))
        reader.feed_data(encode(dict(protocol=VERSION, method="execution.start",
                                     key="start", payload={"roundKey": "r1", "inputs": {}})))
        for _ in range(20):
            await asyncio.sleep(0)
            if any(frame["method"] == "error" for frame in writer.frames):
                break
        reader.feed_eof()
        await agent.serve_streams(reader, writer)

        failures = [frame for frame in writer.frames if frame["method"] == "error"]
        self.assertEqual(len(failures), 1)
        self.assertEqual(failures[0]["payload"], {"code": "ADAPTER_EXECUTION_FAILED"})
        self.assertEqual(failures[0]["replyTo"], "start")
        self.assertFalse(any(frame["method"] == "execution.finished"
                             for frame in writer.frames))

    async def test_decorator_rejects_sync_handler(self):
        agent = AgentAdapter(name="fake", version="1")
        with self.assertRaises(ValueError):
            agent.run(lambda request, context: None)
