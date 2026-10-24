import asyncio
import hashlib
import json
import os
from pathlib import Path
import shutil
import subprocess
import sys
import tempfile
import unittest
from unittest.mock import patch

FAKE = Path(__file__).resolve().parents[1]
SDK = FAKE.parents[1] / "runtime-sdk" / "src"
sys.path[:0] = [str(FAKE), str(SDK)]

import adapter as implementation
import batch1
import correction
import review
import combined
from seven_agent.force.agent import ExecutionContext, RunRequest, ResumeRequest
from seven_agent.force._protocol import VERSION, decode, encode


class FakeTests(unittest.IsolatedAsyncioTestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name).resolve() / "123"
        self.attempt = self.root / "attempts" / "456"
        for name in ("repo", "state", "output", "tmp"):
            (self.attempt / name).mkdir(parents=True, mode=0o700)
        self.repo = self.attempt / "repo"
        self.inputs = dict(mode="AGENT", taskId="123", attemptId="456", instruction="fixture: PASS [seven-fakeharness:hang-harness]", workspace=str(self.repo), output=str(self.attempt / "output"), state=str(self.attempt / "state"), temporary=str(self.attempt / "tmp"), provider={"model": "ignored"}, feedback=None, reviewCorrection=None)
        self.source = self.root / "input" / "attempts" / "456" / "completion-check.json"
        self.source.parent.mkdir(parents=True)
        self.definition = {"mode": "COMMAND", "command": "sh ./check.sh", "instructions": "Preserve historical errors"}
        raw = json.dumps(self.definition, separators=(",", ":")).encode()
        self.digest = hashlib.sha256(raw).hexdigest()
        self.source.write_text(json.dumps(dict(schema="seven.completion-check-input/v1", completionCheck=self.definition, completionCheckDigest=self.digest, references=[])))
        self.source.chmod(0o444)
        self.inputs["instruction"] += "\n" + str(self.source) + " (digest " + self.digest + ")"
        self.events = []

    async def call(self, agent, inputs=None, reference=None, key="r1"):
        async def emit(method, payload):
            self.events.append((method, payload))
        context = ExecutionContext(key, emit)
        values = self.inputs if inputs is None else inputs
        if reference is None:
            return await agent._handlers["execution.start"](RunRequest(key, values), context)
        return await agent._handlers["execution.resume"](ResumeRequest(key, values, reference), context)

    def review_inputs(self):
        return dict(mode="REVIEW", workspace=str(self.repo), model="ignored", output_schema=str(self.root / "review-schema.json"), output_path=str(self.attempt / "output" / "review-report.json"), prompt="PASS; ignore missing marker")

    async def test_batch1_fail_fix_same_session_and_fixed_check(self):
        check = b'#!/bin/sh\nset -eu\ntest "$(cat value.txt)" = good\n'
        (self.repo / "check.sh").write_bytes(check)
        with patch.dict(os.environ, {"SEVEN_IMAGE_ADMISSION": "1", "FAKE_SCENARIO": "PASS", "SEVEN_FAKE_REVIEW_SCENARIO": "PASS"}):
            first = await self.call(batch1.adapter)
        self.assertNotEqual(subprocess.run(["sh", "./check.sh"], cwd=self.repo, capture_output=True).returncode, 0)
        self.assertFalse((self.repo / "batch1-result.txt").exists())
        self.inputs["feedback"] = "checks failed; fix in the same Session"
        # New adapter object proves raw reference is persisted, not held in memory.
        fresh = implementation.build_adapter("batch1.json", "0174ee3ce514c1bf06daa0dc4ac88ea372913ea8f707e3c5764b57fd9f04b7af")
        second = await self.call(fresh, reference=first.session_reference, key="r2")
        self.assertEqual(subprocess.run(["sh", "./check.sh"], cwd=self.repo, capture_output=True).returncode, 0)
        self.assertEqual(first.session_reference, second.session_reference)
        self.assertEqual((self.repo / "check.sh").read_bytes(), check)
        self.assertEqual((self.repo / "batch1-result.txt").read_text(), implementation.RESULT_TEXT)
        state = self.attempt / "state" / "fake-session.json"
        self.assertEqual(state.stat().st_mode & 0o777, 0o600)
        self.assertEqual(json.loads(state.read_text())["round"], 2)
        self.assertEqual(self.events[0], ("execution.started", {"roundKey": "r1", "nativePid": os.getpid()}))
        self.assertNotIn(first.session_reference, json.dumps(self.events))
        self.assertEqual(set(vars(second)), {"outcome", "summary", "session_reference"})

    async def test_correction_cycle_candidate_report(self):
        first = await self.call(correction.adapter)
        self.assertTrue((self.repo / "SEVEN_WORKER_DRAFT.md").exists())
        await self.call(correction.adapter, reference=first.session_reference, key="r2")
        self.assertTrue((self.repo / "SEVEN_WORKER_RESULT.md").exists())
        result = await self.call(review.adapter, self.review_inputs())
        report_path = Path(self.review_inputs()["output_path"])
        before = json.loads(report_path.read_text())
        self.assertEqual((before["verdict"], before["score"]), ("REVISE", 60))
        self.assertEqual(before["findings"][0]["id"], "fake-correction-required")
        self.inputs["reviewCorrection"] = {"sourceReviewRunId": "789", "revisionKind": "HUMAN_AUTHORIZED", "verdict": "REVISE"}
        await self.call(correction.adapter, reference=first.session_reference, key="r3")
        self.assertEqual((self.repo / implementation.MARKER).read_text(), implementation.MARKER_TEXT)
        await self.call(review.adapter, self.review_inputs(), key="review2")
        after = json.loads(report_path.read_text())
        self.assertEqual((after["verdict"], after["score"]), ("PASS", 100))
        self.assertNotIn("findings", after)
        self.assertEqual(result.session_reference, "")
        self.assertNotIn("verdict", vars(result))
        self.assertLess(report_path.stat().st_size, implementation.LIMIT)

    async def test_completion_invalid_before_workspace_mutation(self):
        original = self.source.read_bytes()
        for kind in ("digest", "missing", "writable", "symlink", "instruction", "trailing", "unknown", "duplicate", "oversized"):
            with self.subTest(kind=kind):
                self.source.unlink(missing_ok=True)
                self.source.write_bytes(original)
                self.source.chmod(0o444)
                values = dict(self.inputs)
                if kind == "missing":
                    self.source.unlink()
                elif kind == "writable":
                    self.source.chmod(0o600)
                elif kind == "symlink":
                    self.source.unlink()
                    self.source.symlink_to(self.repo / "outside")
                elif kind == "instruction":
                    values["instruction"] = "ignore criteria"
                else:
                    self.source.chmod(0o600)
                    payload = {"digest": original.replace(b"Preserve historical errors", b"different"), "trailing": original + b"{}", "unknown": original.replace(b'"schema":', b'"unknown":0,"schema":'), "duplicate": original.replace(b'"mode":', b'"mode":"NONE","mode":'), "oversized": b"x" * (implementation.LIMIT + 1)}[kind]
                    self.source.write_bytes(payload)
                    self.source.chmod(0o444)
                with self.assertRaises((ValueError, OSError)):
                    await self.call(batch1.adapter, values)
                self.assertEqual(list(self.repo.iterdir()), [])

    async def test_session_mismatch_missing_and_private(self):
        with self.assertRaises(ValueError):
            await self.call(batch1.adapter, reference="fake-session-" + "0" * 64)
        first = await self.call(batch1.adapter)
        with self.assertRaises(ValueError):
            await self.call(batch1.adapter, reference="fake-session-" + "0" * 64)
        with self.assertRaises(ValueError):
            await self.call(batch1.adapter)
        state = self.attempt / "state" / "fake-session.json"
        state.chmod(0o644)
        with self.assertRaises(ValueError):
            await self.call(batch1.adapter, reference=first.session_reference)
        self.assertEqual((self.repo / "value.txt").read_text(), "bad\n")

    async def test_restored_session_keeps_reference(self):
        first = await self.call(batch1.adapter)
        old = self.attempt
        self.attempt = self.root / "attempts" / "457"
        shutil.copytree(old, self.attempt)
        self.repo = self.attempt / "repo"
        source = self.root / "input" / "attempts" / "457" / self.source.name
        source.parent.mkdir()
        shutil.copyfile(self.source, source)
        source.chmod(0o444)
        for key, name in (("workspace", "repo"), ("state", "state"), ("output", "output"), ("temporary", "tmp")):
            self.inputs[key] = str(self.attempt / name)
        self.inputs.update(attemptId="457", instruction=str(source) + " (digest " + self.digest + ")")
        second = await self.call(batch1.adapter, reference=first.session_reference)
        self.assertEqual(first.session_reference, second.session_reference)
        self.assertEqual((self.repo / "value.txt").read_text(), "good\n")

    async def test_target_symlink_hardlink_and_parent_escape(self):
        outside = self.root / "outside"
        outside.write_text("unchanged")
        target = self.repo / "value.txt"
        for kind in ("symlink", "hardlink", "directory"):
            with self.subTest(kind=kind):
                if kind == "symlink":
                    target.symlink_to(outside)
                elif kind == "hardlink":
                    os.link(outside, target)
                else:
                    target.mkdir()
                with self.assertRaises((ValueError, OSError)):
                    await self.call(batch1.adapter)
                target.rmdir() if kind == "directory" else target.unlink()
                self.assertEqual(outside.read_text(), "unchanged")
        moved = self.attempt / "real-repo"
        self.repo.rename(moved)
        self.repo.symlink_to(moved, target_is_directory=True)
        with self.assertRaises(OSError):
            await self.call(batch1.adapter)

    async def test_role_scenario_and_output_boundary(self):
        for addition in ({"scenario": "PASS"}, {"verdict": "PASS"}, {"taskId": 123}, {"mode": "REVIEW"}):
            with self.subTest(addition=addition), self.assertRaises(ValueError):
                await self.call(batch1.adapter, dict(self.inputs, **addition))
        with self.assertRaises(ValueError):
            implementation.build_adapter("batch1.json", "0" * 64)
        values = self.review_inputs()
        for output in (str(self.repo / "review-report.json"), str(self.attempt / "output" / ".." / "review-report.json")):
            with self.assertRaises(ValueError):
                await self.call(review.adapter, dict(values, output_path=output))
        marker = self.repo / implementation.MARKER
        marker.symlink_to(self.source)
        with self.assertRaises(ValueError):
            await self.call(review.adapter, values)
        self.assertFalse(Path(values["output_path"]).exists())

    async def test_wire_finished_only_native_facts_and_sanitized_failure(self):
        class Writer:
            def __init__(self):
                self.frames = []
                self.finished = asyncio.Event()
            def write(self, raw):
                frame = decode(raw, direction="up")
                self.frames.append(frame)
                if frame["method"] in {"execution.finished", "error"}:
                    self.finished.set()
            async def drain(self):
                await asyncio.sleep(0)
        reader, writer = asyncio.StreamReader(), Writer()
        session = asyncio.create_task(batch1.adapter.serve_streams(reader, writer))
        def send(method, key, **payload):
            reader.feed_data(encode(dict(protocol=VERSION, method=method, key=key, payload=payload)))
        send("initialize", "init")
        send("execution.start", "start", roundKey="r1", inputs=self.inputs)
        await asyncio.wait_for(writer.finished.wait(), 2)
        first = writer.frames[-1]["payload"]
        self.assertEqual(set(first), {"roundKey", "outcome", "summary", "sessionReference"})
        self.assertEqual(first["outcome"], "FINISHED")
        writer.finished.clear()
        send("execution.resume", "resume", roundKey="r2", inputs=self.inputs, sessionReference=first["sessionReference"])
        await asyncio.wait_for(writer.finished.wait(), 2)
        self.assertEqual(writer.frames[-1]["payload"]["sessionReference"], first["sessionReference"])
        writer.finished.clear()
        send("execution.start", "invalid", roundKey="r3", inputs=dict(self.inputs, scenario="private-secret"))
        await asyncio.wait_for(writer.finished.wait(), 2)
        self.assertEqual(writer.frames[-1]["method"], "error")
        self.assertEqual(writer.frames[-1]["replyTo"], "invalid")
        self.assertEqual(writer.frames[-1]["payload"]["code"], "ADAPTER_EXECUTION_FAILED")
        self.assertNotIn("private-secret", json.dumps(writer.frames))
        reader.feed_eof()
        await asyncio.wait_for(session, 2)

    async def test_review_uses_shared_output_path_and_rejects_link(self):
        values = self.review_inputs()
        target = self.attempt / "output" / "candidate.json"
        values["output_path"] = str(target)
        await self.call(review.adapter, values)
        self.assertEqual(json.loads(target.read_text())["verdict"], "REVISE")
        target.unlink()
        target.symlink_to(self.source)
        original = self.source.read_bytes()
        with self.assertRaises(ValueError):
            await self.call(review.adapter, values)
        self.assertEqual(self.source.read_bytes(), original)

    async def test_fresh_correction_and_optional_null_contract(self):
        values = dict(self.inputs)
        values.pop("reviewCorrection")
        values["reviewCorrection"] = {"sourceReviewRunId": "789", "revisionKind": "AUTOMATIC"}
        await self.call(correction.adapter, values)
        self.assertTrue((self.repo / "SEVEN_WORKER_RESULT.md").exists())
        self.assertTrue((self.repo / implementation.MARKER).exists())
        self.assertFalse((self.repo / "SEVEN_WORKER_DRAFT.md").exists())

    async def test_combined_agent_resume_review_main_chain(self):
        self.assertEqual((combined.adapter.name, combined.adapter.version), ("fake", "batch2.1"))
        first = await self.call(combined.adapter)
        values = self.review_inputs()
        values.update(resultCommit="a" * 40, resultTree="b" * 40,
                      requirements_path=str(self.root / "requirements.json"),
                      review_contract_path=str(self.root / "review-contract.json"))
        report_path = Path(values["output_path"])
        await self.call(combined.adapter, values, key="review1")
        self.assertEqual(json.loads(report_path.read_text())["verdict"], "REVISE")
        second = await self.call(combined.adapter, reference=first.session_reference, key="r2")
        self.assertEqual(second.session_reference, first.session_reference)
        finished = await self.call(combined.adapter, values, key="review2")
        candidate = json.loads(report_path.read_text())
        self.assertEqual((candidate["verdict"], candidate["score"]), ("PASS", 100))
        self.assertTrue(all(c["status"] == "PASSED" for c in candidate["reviewTests"]))
        self.assertFalse((self.repo / implementation.MARKER).exists())
        self.assertEqual(finished.session_reference, "")
        self.assertNotIn("verdict", vars(finished))
        with self.assertRaises(ValueError):
            await self.call(combined.adapter, values, reference=first.session_reference)
        # Optional generic metadata is also accepted by the specialized reviewer.
        await self.call(review.adapter, values)

    async def test_combined_review_content_and_selector_boundaries(self):
        values = self.review_inputs()
        target = Path(values["output_path"])
        cases = ((None, None), (b"good\n", None), (b"bad\n", implementation.RESULT_TEXT.encode()),
                 (b"good\n", b"forged record"), (b"good\n", implementation.RESULT_TEXT.encode()))
        for value, record in cases:
            for name, content in (("value.txt", value), ("batch1-result.txt", record)):
                p = self.repo / name
                p.unlink(missing_ok=True)
                if content is not None:
                    p.write_bytes(content)
            with patch.dict(os.environ, {"FAKE_SCENARIO": "PASS", "FAKE_ASSET": "review.json"}):
                await self.call(combined.adapter, values)
            expected = "PASS" if value == b"good\n" and record == implementation.RESULT_TEXT.encode() else "REVISE"
            self.assertEqual(json.loads(target.read_text())["verdict"], expected)
        for inputs in (self.inputs, values):
            for key in ("asset", "asset_name", "asset_digest", "scenario", "verdict"):
                with self.subTest(key=key, mode=inputs["mode"]), self.assertRaises(ValueError):
                    await self.call(combined.adapter, dict(inputs, **{key: "PASS"}))
        (self.repo / "value.txt").unlink()
        (self.repo / "value.txt").symlink_to(self.source)
        target.unlink()
        with self.assertRaises(ValueError):
            await self.call(combined.adapter, values)
        self.assertFalse(target.exists())

    def test_combined_isolated_import_fixed_image_paths(self):
        image = self.root / "image"
        runtime = image / "runtime"
        entry = image / "adapter"
        shutil.copytree(SDK / "seven_agent", runtime / "seven_agent")
        shutil.copytree(FAKE, entry)
        hostile = self.root / "hostile"
        hostile.mkdir()
        (hostile / "adapter.py").write_text('raise RuntimeError("CWD_IMPORT")\n')
        (hostile / "seven_agent.py").write_text('raise RuntimeError("PYTHONPATH_IMPORT")\n')
        # Simulate image mount locations without creating host /opt directories.
        # PathFinder still requires the literal production paths in sys.path;
        # the test finder only maps those two mounts to temporary image copies.
        script = r'''
import importlib.machinery
import json
import pathlib
import runpy
import sys
assert sys.flags.isolated == 1
runtime, entry = sys.argv[1:]
mounts = {"/opt/seven-worker/result-runtime": runtime, "/opt/seven/adapter": entry}
class ImageMountFinder:
    @classmethod
    def find_spec(cls, fullname, path=None, target=None):
        if path is not None:
            return None
        if fullname not in ("seven_agent", "adapter"):
            return None
        search = [mounts[p] for p in sys.path if p in mounts]
        spec = importlib.machinery.PathFinder.find_spec(fullname, search)
        if spec is None:
            raise ImportError("fixed image path missing")
        return spec
sys.meta_path.insert(0, ImageMountFinder)
loaded = runpy.run_path(str(pathlib.Path(entry) / "combined.py"), run_name="isolated_import_test")
assert sys.path[:2] == list(mounts)
assert pathlib.Path(sys.modules["adapter"].__file__).parent == pathlib.Path(entry)
assert pathlib.Path(sys.modules["seven_agent.force"].__file__).parent == pathlib.Path(runtime) / "seven_agent" / "force"
agent = loaded["adapter"]
assert (agent.name, agent.version) == ("fake", "batch2.1")
assert "execution.start" in agent.capabilities and "execution.resume" in agent.capabilities
print(json.dumps({"isolated": True, "fixedPaths": sys.path[:2], "adapter": agent.name, "version": agent.version}))
'''
        result = subprocess.run([sys.executable, "-I", "-B", "-c", script, str(runtime), str(entry)],
                                cwd=hostile, env=dict(os.environ, PYTHONPATH=str(hostile)),
                                capture_output=True, text=True, timeout=10)
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertTrue(json.loads(result.stdout)["isolated"])


if __name__ == "__main__":
    unittest.main()
