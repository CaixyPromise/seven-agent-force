"""Fixed Fake safe-point and cold resume conformance over real stdio."""
import asyncio
import copy
import hashlib
import json
import os
from pathlib import Path
import runpy
import shutil
import subprocess
import sys
import unittest

import test_fake
from test_fake import FAKE, SDK
from seven_agent.force._protocol import VERSION, decode, encode


class Batch3Tests(unittest.IsolatedAsyncioTestCase):
    def setUp(self):
        test_fake.FakeTests.setUp(self)
        self.state_path = self.attempt / "state" / "codex" / "fake-session.json"

    async def start(self):
        # The stdlib event loop may use a local socketpair, but network is forbidden.
        program = '''
import runpy, sys
def audit(event, args):
    if event in ("socket.connect", "socket.getaddrinfo", "socket.bind"):
        raise RuntimeError("NETWORK_FORBIDDEN")
sys.addaudithook(audit)
runpy.run_path(sys.argv[1], run_name="__main__")
'''
        process = await asyncio.create_subprocess_exec(sys.executable, "-B", "-c", program, str(FAKE / "batch3.py"),
            stdin=asyncio.subprocess.PIPE, stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE,
            env=dict(os.environ, PYTHONPATH=os.pathsep.join((str(SDK), str(FAKE))),
                     FAKE_SCENARIO="PASS", SEVEN_FAKE_BATCH="batch2"))
        self.addAsyncCleanup(self.close, process)
        await self.send(process, "initialize", "init", {})
        ready = await self.until(process, "ready")
        manifest = json.loads((FAKE.parents[1] / "images/fake-harness-batch3-manifest.json").read_text())
        self.assertEqual(ready["payload"]["capabilities"], manifest["adapter"]["capabilities"])
        self.assertEqual(ready["payload"]["version"], manifest["adapter"]["version"])
        self.assertEqual(ready["payload"]["name"], manifest["adapter"]["id"])
        return process

    async def close(self, process):
        if process.returncode is None:
            process.stdin.close()
            try:
                await asyncio.wait_for(process.wait(), 3)
            except asyncio.TimeoutError:
                process.kill()
                await process.wait()
                self.fail("Fake did not stop")
        self.assertEqual(await process.stderr.read(), b"")
        self.assertEqual(process.returncode, 0)

    async def send(self, process, method, key, payload):
        process.stdin.write(encode(dict(protocol=VERSION, method=method, key=key, payload=payload)))
        await process.stdin.drain()

    async def until(self, process, method):
        while True:
            raw = await asyncio.wait_for(process.stdout.readline(), 3)
            self.assertTrue(raw, "Unexpected EOF")
            frame = decode(raw, direction="up")
            self.events.append(frame)
            if frame["method"] == method:
                return frame

    async def run_question(self, process, round_key="r"):
        await self.send(process, "execution.start", "run", {"roundKey": round_key, "inputs": self.inputs})
        frame = await self.until(process, "interaction.request")
        self.assertEqual(frame["payload"]["interactionKey"], "fake:question")
        self.assertEqual(frame["replyTo"], "run")
        self.assertFalse((self.attempt / "state" / "session.json").exists())

    def answer(self, choice="alpha"):
        return {"interactionKey": "fake:question", "responseId": "101", "answers": [
            {"questionKey": "result", "selectedOptionKeys": [choice], "text": ""}]}

    async def supplement(self, process, round_key="r", text="Keep this exact supplement.", key="m"):
        payload = {"roundKey": round_key, "messageKey": key, "text": text}
        await self.send(process, "user.message", "msg:" + key, payload)
        receipt = await self.until(process, "message.received")
        self.assertEqual(receipt["payload"]["status"], "RECEIVED")
        return payload

    def session(self):
        return json.loads(self.state_path.read_text())

    def restore(self):
        """Simulate checkpoint restoration into another Attempt, preserving native bytes."""
        new_attempt = self.root / "attempts" / "789"
        for name in ("repo", "state", "output", "tmp"):
            (new_attempt / name).mkdir(parents=True, mode=0o700)
        shutil.copytree(self.attempt / "state" / "codex", new_attempt / "state" / "codex")
        self.attempt = new_attempt
        self.repo = new_attempt / "repo"
        self.state_path = new_attempt / "state" / "codex" / "fake-session.json"
        self.inputs = dict(self.inputs, attemptId="789", workspace=str(self.repo),
                           state=str(new_attempt / "state"), output=str(new_attempt / "output"),
                           temporary=str(new_attempt / "tmp"))

    async def test_answer_then_supplement_and_duplicate_delivery(self):
        process = await self.start()
        await self.run_question(process)
        self.assertEqual([f["method"] for f in self.events if f["method"] != "ready"],
                         ["execution.started", "activity", "activity", "message", "plan", "interaction.request"])
        self.assertEqual([f["payload"]["status"] for f in self.events if f["method"] == "activity"],
                         ["STARTED", "COMPLETED"])
        await self.send(process, "interaction.respond", "answer", {"roundKey": "r", **self.answer()})
        await self.until(process, "interaction.received")
        self.assertFalse((self.repo / "batch3-result.txt").exists())
        self.assertFalse(any(f["method"] == "activity" and f["payload"]["status"] == "SUCCEEDED" for f in self.events))
        await self.supplement(process)
        while not any(f["method"] == "activity" and f["payload"]["status"] == "SUCCEEDED" for f in self.events):
            await self.until(process, "activity")
        self.assertTrue(self.session()["resultWritten"])
        self.assertEqual((self.repo / "batch3-result.txt").read_text(), "Fixed answer: alpha\n")
        self.assertEqual((self.repo / "batch3-supplements.txt").read_text(), "Keep this exact supplement.\n")
        result = await self.until(process, "execution.finished")
        self.assertEqual(result["payload"]["outcome"], "FINISHED")
        self.assertEqual(result["payload"]["sessionReference"], self.session()["sessionId"])
        self.assertEqual((self.repo / "batch3-result.txt").read_text(), "Fixed answer: alpha\n")
        self.assertEqual((self.repo / "batch3-supplements.txt").read_text(), "Keep this exact supplement.\n")
        before = (self.state_path.read_bytes(), (self.repo / "batch3-result.txt").stat().st_mtime_ns)
        await self.send(process, "interaction.respond", "answer-retry", {"roundKey": "r", **self.answer()})
        await self.until(process, "interaction.received")
        await self.supplement(process)
        self.assertEqual(before, (self.state_path.read_bytes(), (self.repo / "batch3-result.txt").stat().st_mtime_ns))
        self.assertEqual(len(self.session()["supplements"]), 1)
        self.assertEqual([f["payload"]["status"] for f in self.events if f["method"] == "activity"],
                         ["STARTED", "COMPLETED", "STARTED", "SUCCEEDED"])
        reference = self.session()["sessionId"]
        await self.close(process)
        fresh = await self.start()
        boundary = len(self.events)
        await self.send(fresh, "execution.resume", "resume", {
            "roundKey": "r2", "sessionReference": reference, "inputs": self.inputs})
        replay = await self.until(fresh, "execution.finished")
        self.assertEqual(replay["payload"]["sessionReference"], reference)
        self.assertEqual((self.repo / "batch3-result.txt").stat().st_mtime_ns, before[1])
        self.assertFalse(any(f["method"] == "interaction.request" for f in self.events[boundary:]))
        self.assertFalse(any(f["method"] == "activity" and f["payload"]["activityKey"] == "fake:record"
                             for f in self.events[boundary:]))

    async def test_failed_result_write_never_emits_success(self):
        process = await self.start()
        await self.run_question(process)
        (self.repo / "batch3-result.txt").mkdir()
        await self.send(process, "interaction.respond", "answer", {"roundKey": "r", **self.answer()})
        await self.until(process, "interaction.received")
        await self.send(process, "user.message", "msg:m", {"roundKey": "r", "messageKey": "m", "text": "Persist this"})
        finished = await self.until(process, "execution.finished")
        self.assertEqual(finished["payload"]["outcome"], "FAILED")
        self.assertFalse(self.session()["resultWritten"])
        self.assertFalse(any(f["method"] == "activity" and f["payload"]["status"] == "SUCCEEDED" for f in self.events))

    async def test_hotwait_suspend_release_and_fresh_process_saved_answer(self):
        process = await self.start()
        await self.run_question(process)
        await self.supplement(process, text="Saved before hotwait.")
        await self.send(process, "execution.suspend", "pause", {"roundKey": "r"})
        suspended = await self.until(process, "execution.suspended")
        reference = suspended["payload"]["sessionReference"]
        self.assertEqual(self.session()["phase"], "SUSPENDED")
        self.assertIsNone(self.session()["answer"])
        before = self.state_path.read_bytes()
        await self.send(process, "user.message", "late", {"roundKey": "r", "messageKey": "late", "text": "No mutations"})
        self.assertEqual((await self.until(process, "error"))["payload"]["code"], "ADAPTER_ROUND_NOT_RUNNING")
        await self.send(process, "execution.suspend", "pause-retry", {"roundKey": "r"})
        self.assertEqual((await self.until(process, "execution.suspended"))["payload"]["sessionReference"], reference)
        self.assertEqual(self.state_path.read_bytes(), before)
        self.assertFalse((self.repo / "batch3-result.txt").exists())
        self.assertFalse(any(f["method"] == "execution.finished" for f in self.events))
        await self.close(process)  # Original process is gone before restore.
        self.restore()
        restored = await self.start()
        boundary = len(self.events)
        await self.send(restored, "execution.resume", "resume", {
            "roundKey": "r2", "sessionReference": reference,
            "inputs": dict(self.inputs, interactionResponse=self.answer("beta"),
                           interactionQuestionRequest=self.session()["question"])})
        await self.until(restored, "execution.started")
        self.assertFalse((self.repo / "batch3-result.txt").exists())
        self.assertIsNone(self.session()["answer"])
        await self.send(restored, "interaction.respond", "wrong-cold", {"roundKey": "r2", **self.answer("alpha")})
        self.assertEqual((await self.until(restored, "error"))["payload"]["code"], "ADAPTER_IDEMPOTENCY_CONFLICT")
        self.assertFalse((self.repo / "batch3-result.txt").exists())
        self.assertFalse(any(f["method"] in {"interaction.request", "interaction.received", "execution.finished"} for f in self.events[boundary:]))
        await self.send(restored, "interaction.respond", "cold-delivery", {"roundKey": "r2", **self.answer("beta")})
        receipt = await self.until(restored, "interaction.received")
        self.assertEqual(receipt["replyTo"], "cold-delivery")
        self.assertEqual(receipt["payload"], {"roundKey": "r2", "interactionKey": "fake:question", "responseId": "101", "status": "RECEIVED"})
        finished = await self.until(restored, "execution.finished")
        self.assertEqual(finished["payload"]["outcome"], "FINISHED")
        self.assertEqual(finished["payload"]["sessionReference"], reference)
        self.assertEqual(self.session()["sessionId"], reference)
        self.assertEqual(self.session()["round"], 2)
        self.assertEqual(self.session()["answer"], self.answer("beta"))
        self.assertEqual((self.repo / "batch3-result.txt").read_text(), "Fixed answer: beta\n")
        self.assertEqual((self.repo / "batch3-supplements.txt").read_text(), "Saved before hotwait.\n")
        self.assertFalse(any(f["method"] == "interaction.request" for f in self.events[boundary:]))
        before = (self.state_path.read_bytes(), (self.repo / "batch3-result.txt").stat().st_mtime_ns)
        await self.send(restored, "interaction.respond", "cold-retry", {"roundKey": "r2", **self.answer("beta")})
        self.assertEqual((await self.until(restored, "interaction.received"))["payload"], receipt["payload"])
        self.assertEqual(before, (self.state_path.read_bytes(), (self.repo / "batch3-result.txt").stat().st_mtime_ns))

    async def test_buffered_answer_suspend_preserves_accepted_answer(self):
        process = await self.start()
        await self.run_question(process)
        for method, key, payload in (
            ("interaction.respond", "answer", {"roundKey": "r", **self.answer()}),
            ("execution.suspend", "pause", {"roundKey": "r"})):
            process.stdin.write(encode(dict(protocol=VERSION, method=method, key=key, payload=payload)))
        await process.stdin.drain()
        paused = await self.until(process, "execution.suspended")
        reference = paused["payload"]["sessionReference"]
        self.assertEqual(self.session()["answer"], self.answer())
        await self.close(process)
        restored = await self.start()
        boundary = len(self.events)
        await self.send(restored, "execution.resume", "resume", {
            "roundKey": "r2", "sessionReference": reference, "inputs": self.inputs})
        await self.until(restored, "execution.started")
        await self.supplement(restored, round_key="r2")
        result = await self.until(restored, "execution.finished")
        self.assertEqual(result["payload"]["outcome"], "FINISHED")
        self.assertEqual(result["payload"]["sessionReference"], reference)
        self.assertFalse(any(f["method"] == "interaction.request" for f in self.events[boundary:]))

    async def test_cancel_waiting_and_reject_runtime_fixture_selection(self):
        process = await self.start()
        await self.run_question(process)
        before = self.state_path.read_bytes()
        await self.send(process, "execution.interrupt", "cancel", {"roundKey": "r"})
        self.assertEqual((await self.until(process, "execution.finished"))["payload"]["outcome"], "CANCELLED")
        self.assertFalse((self.repo / "batch3-result.txt").exists())
        self.assertEqual(before, self.state_path.read_bytes())
        for field in ("scenario", "fixture", "SEVEN_FAKE_BATCH"):
            await self.send(process, "execution.start", field, {
                "roundKey": field, "inputs": dict(self.inputs, **{field: "PASS"})})
            failure = await self.until(process, "error")
            self.assertEqual(failure["replyTo"], field)
            self.assertEqual(failure["payload"]["code"], "ADAPTER_EXECUTION_FAILED")
        self.assertEqual(before, self.state_path.read_bytes())

    async def test_cold_answer_conflict_fails_before_session_write(self):
        process = await self.start()
        await self.run_question(process)
        await self.send(process, "interaction.respond", "answer", {"roundKey": "r", **self.answer()})
        await self.until(process, "interaction.received")
        await self.send(process, "execution.suspend", "pause", {"roundKey": "r"})
        reference = (await self.until(process, "execution.suspended"))["payload"]["sessionReference"]
        await self.close(process)
        before = self.state_path.read_bytes()
        restored = await self.start()
        await self.send(restored, "execution.resume", "resume", {
            "roundKey": "r2", "sessionReference": reference,
            "inputs": dict(self.inputs, interactionResponse=self.answer("beta"),
                           interactionQuestionRequest=self.session()["question"])})
        failure = await self.until(restored, "error")
        self.assertEqual(failure["replyTo"], "resume")
        self.assertEqual(failure["payload"]["code"], "ADAPTER_EXECUTION_FAILED")
        self.assertEqual(before, self.state_path.read_bytes())

    async def test_cold_requires_complete_trusted_question_before_session_write(self):
        process = await self.start()
        await self.run_question(process)
        await self.send(process, "execution.suspend", "pause", {"roundKey": "r"})
        reference = (await self.until(process, "execution.suspended"))["payload"]["sessionReference"]
        await self.close(process)
        question = self.session()["question"]
        changed = copy.deepcopy(question)
        changed["questions"][0]["title"] = "Altered host question"
        reduced = copy.deepcopy(question)
        del reduced["questions"][0]["required"]
        before = self.state_path.read_bytes()
        for cold in (
            {"interactionResponse": self.answer()},
            {"interactionQuestionRequest": question},
            {"interactionQuestionRequest": None, "interactionResponse": None},
            {"interactionQuestionRequest": changed, "interactionResponse": self.answer()},
            {"interactionQuestionRequest": reduced, "interactionResponse": self.answer()},
        ):
            fresh = await self.start()
            await self.send(fresh, "execution.resume", "resume", {"roundKey": "r2", "sessionReference": reference, "inputs": dict(self.inputs, **cold)})
            failure = await self.until(fresh, "error")
            self.assertEqual(failure["replyTo"], "resume")
            self.assertEqual(failure["payload"]["code"], "ADAPTER_EXECUTION_FAILED")
            self.assertEqual(self.state_path.read_bytes(), before)
            await self.close(fresh)

    async def test_cold_wait_is_interruptible_without_automatic_receipt(self):
        process = await self.start()
        await self.run_question(process)
        await self.supplement(process)
        await self.send(process, "execution.suspend", "pause", {"roundKey": "r"})
        reference = (await self.until(process, "execution.suspended"))["payload"]["sessionReference"]
        await self.close(process)
        self.restore()
        fresh = await self.start()
        boundary = len(self.events)
        await self.send(fresh, "execution.resume", "resume", {"roundKey": "r2", "sessionReference": reference,
            "inputs": dict(self.inputs, interactionResponse=self.answer(), interactionQuestionRequest=self.session()["question"])})
        await self.until(fresh, "execution.started")
        await self.send(fresh, "execution.interrupt", "interrupt", {"roundKey": "r2"})
        result = await self.until(fresh, "execution.finished")
        self.assertEqual(result["payload"]["outcome"], "CANCELLED")
        self.assertFalse(any(f["method"] in {"interaction.request", "interaction.received"} for f in self.events[boundary:]))
        self.assertFalse((self.repo / "batch3-result.txt").exists())
        self.assertEqual(self.session()["sessionId"], reference)
        self.assertIsNone(self.session()["answer"])

    async def test_buffered_fresh_resume_and_normal_cold_delivery(self):
        process = await self.start()
        await self.run_question(process)
        await self.supplement(process)
        await self.send(process, "execution.suspend", "pause", {"roundKey": "r"})
        reference = (await self.until(process, "execution.suspended"))["payload"]["sessionReference"]
        await self.close(process)
        self.restore()
        fresh = await self.start()
        boundary = len(self.events)
        restored_inputs = dict(self.inputs, interactionResponse=self.answer(), interactionQuestionRequest=self.session()["question"])
        for method, key, payload in (
            ("execution.resume", "resume", {"roundKey": "r2", "sessionReference": reference, "inputs": restored_inputs}),
            ("interaction.respond", "buffered-answer", {"roundKey": "r2", **self.answer()}),
        ):
            fresh.stdin.write(encode(dict(protocol=VERSION,method=method,key=key,payload=payload)))
        await fresh.stdin.drain()
        finished = await self.until(fresh, "execution.finished")
        self.assertEqual(finished["payload"]["outcome"], "FINISHED")
        self.assertEqual(finished["payload"]["sessionReference"], reference)
        frames = self.events[boundary:]
        self.assertFalse(any(f["method"] in {"interaction.request", "error"} for f in frames))
        receipt = next(f for f in frames if f["method"] == "interaction.received")
        self.assertEqual(receipt["replyTo"], "buffered-answer")
        self.assertEqual(receipt["payload"]["responseId"], "101")
        self.assertEqual(receipt["payload"]["roundKey"], "r2")
        self.assertLess(frames.index(receipt), frames.index(finished))

    async def test_session_symlink_rejected(self):
        (self.attempt / "state" / "codex").symlink_to(self.repo)
        process = await self.start()
        await self.send(process, "execution.start", "run", {"roundKey": "r", "inputs": self.inputs})
        failure = await self.until(process, "error")
        self.assertEqual(failure["replyTo"], "run")
        self.assertEqual(failure["payload"]["code"], "ADAPTER_EXECUTION_FAILED")
        self.assertEqual(list(self.repo.iterdir()), [])

    async def test_message_suspend_buffer_drains_before_suspended_receipt(self):
        process = await self.start()
        await self.run_question(process)
        for method, key, payload in (
            ("user.message", "msg", {"roundKey": "r", "messageKey": "m", "text": "Saved at final safe point."}),
            ("execution.suspend", "pause", {"roundKey": "r"})):
            process.stdin.write(encode(dict(protocol=VERSION, method=method, key=key, payload=payload)))
        await process.stdin.drain()
        await self.until(process, "execution.suspended")
        receipt = next(f for f in self.events if f["method"] == "message.received")
        self.assertEqual(receipt["payload"]["status"], "RECEIVED")
        self.assertEqual(self.session()["supplements"], [{"messageKey": "m", "text": "Saved at final safe point."}])
        before = self.state_path.read_bytes()
        await self.send(process, "user.message", "retry", {"roundKey": "r", "messageKey": "m", "text": "Saved at final safe point."})
        self.assertEqual((await self.until(process, "message.received"))["payload"]["status"], "RECEIVED")
        self.assertEqual(self.state_path.read_bytes(), before)

    async def test_persisted_supplement_dedup_and_session_bounds(self):
        process = await self.start()
        await self.run_question(process)
        await self.supplement(process, text="Persisted once.")
        await self.send(process, "execution.suspend", "pause", {"roundKey": "r"})
        reference = (await self.until(process, "execution.suspended"))["payload"]["sessionReference"]
        await self.close(process)
        restored = await self.start()
        await self.send(restored, "execution.resume", "resume", {"roundKey": "r2", "sessionReference": reference, "inputs": self.inputs})
        await self.until(restored, "interaction.request")
        await self.supplement(restored, round_key="r2", text="Persisted once.")
        self.assertEqual(len(self.session()["supplements"]), 1)
        # Different envelope in a fresh process still cannot change a saved message.
        await self.send(restored, "user.message", "conflict", {"roundKey": "r2", "messageKey": "m", "text": "Changed"})
        self.assertEqual((await self.until(restored, "error"))["payload"]["code"], "ADAPTER_IDEMPOTENCY_CONFLICT")
        for i in range(3):
            await self.supplement(restored, round_key="r2", text="x" * 16384, key="large:" + str(i))
        await self.send(restored, "user.message", "over-limit", {"roundKey": "r2", "messageKey": "over", "text": "x" * 16384})
        self.assertEqual((await self.until(restored, "message.received"))["payload"]["status"], "REJECTED")
        self.assertEqual(len(self.session()["supplements"]), 4)

    def test_batch3_isolated_import_uses_indexed_modules_and_resources(self):
        image = self.root / "isolated-image"
        runtime, entry = image / "runtime", image / "adapter"
        shutil.copytree(SDK / "seven_agent", runtime / "seven_agent", ignore=shutil.ignore_patterns("__pycache__"))
        entry.mkdir()
        for name in ("adapter.py", "batch3.py", "interaction.py"):
            shutil.copyfile(FAKE / name, entry / name)
        shutil.copytree(FAKE / "fixtures", entry / "fixtures")
        hostile = self.root / "hostile"
        hostile.mkdir()
        (hostile / "interaction.py").write_text('raise RuntimeError("HOSTILE_IMPORT")\n')
        script = '''
import importlib.machinery, pathlib, runpy, sys
runtime, entry = sys.argv[1:]
mounts = {"/opt/seven-worker/result-runtime": runtime, "/opt/seven/adapter": entry}
class Finder:
    @classmethod
    def find_spec(cls, fullname, path=None, target=None):
        if path is not None or fullname not in ("seven_agent", "adapter", "interaction"):
            return None
        return importlib.machinery.PathFinder.find_spec(fullname, [mounts[p] for p in sys.path if p in mounts])
sys.meta_path.insert(0, Finder)
loaded = runpy.run_path(str(pathlib.Path(entry) / "batch3.py"), run_name="image_import")
assert sys.flags.isolated == 1
assert pathlib.Path(sys.modules["interaction"].__file__).parent == pathlib.Path(entry)
assert loaded["adapter"].version == "batch3"
assert "user.message" in loaded["adapter"].capabilities
assert "execution.suspend" in loaded["adapter"].capabilities
'''
        result = subprocess.run([sys.executable, "-I", "-B", "-c", script, str(runtime), str(entry)],
                                cwd=hostile, env=dict(os.environ, PYTHONPATH=str(hostile)), capture_output=True, timeout=5)
        self.assertEqual(result.returncode, 0, result.stderr.decode())

    def test_build_index_contains_entry_modules_and_resources(self):
        assets = self.root / "image"
        assets.mkdir()
        for name in ("adapter.py", "combined.py", "batch3.py", "interaction.py"):
            shutil.copyfile(FAKE / name, assets / name)
        shutil.copytree(FAKE / "fixtures", assets / "fixtures")
        manifest_path = self.root / "manifest.json"
        manifest_source = FAKE.parents[1] / "images/fake-harness-batch3-manifest.json"
        shutil.copyfile(manifest_source, manifest_path)
        builder = runpy.run_path(str(SDK.parent / "build_image_adapter.py"))
        builder["build"](assets, manifest_path, "batch3.py")
        index_bytes = (assets / "source-index.json").read_bytes()
        index = {item["path"]: item["sha256"] for item in json.loads(index_bytes)}
        for name in ("entrypoint", "batch3.py", "interaction.py", "adapter.py", "fixtures/batch3.json"):
            self.assertEqual(index[name], hashlib.sha256((assets / name).read_bytes()).hexdigest())
        manifest = json.loads(manifest_path.read_text())
        self.assertEqual(manifest["adapter"]["sourceSha256"], hashlib.sha256(index_bytes).hexdigest())
        self.assertEqual(manifest["adapter"]["binarySha256"], index["entrypoint"])
        self.assertEqual(manifest["executionKinds"], ["AGENT_ATTEMPT"])
        self.assertIn("python3 -I", (assets / "entrypoint").read_text())
        self.assertIn("batch3.py", (assets / "entrypoint").read_text())
        dockerfile = (FAKE.parents[1] / "images/fake-harness.Dockerfile").read_text()
        self.assertIn("ARG SEVEN_FAKE_BATCH=batch2", dockerfile)
        self.assertIn("batch3) cp", dockerfile)
        self.assertIn("*) exit 1", dockerfile)
        for name in ("batch3.py", "interaction.py"):
            self.assertIn("deploy/workforce/adapters/fake/" + name, dockerfile)
