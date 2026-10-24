import json
import os
from pathlib import Path
import tempfile
import unittest
from unittest.mock import patch

import test_fake
import batch3_review
import batch3_combined
from seven_agent.force.agent import ExecutionContext, RunRequest


class Batch3ReviewTests(unittest.IsolatedAsyncioTestCase):
    async def test_fixed_result_readonly_and_prompt_cannot_choose_verdict(self):
        with tempfile.TemporaryDirectory() as temporary:
            root = Path(temporary).resolve()
            repo = root / "repo"
            repo.mkdir()
            output = root / "output"
            output.mkdir()
            result, supplement = repo / "batch3-result.txt", repo / "batch3-supplements.txt"
            for choice in ("alpha", "beta"):
                result.write_text("Fixed answer: " + choice + "\n")
                supplement.write_text("Private supplement not copied into report\n")
                before = {p.name: p.read_bytes() for p in repo.iterdir()}
                result.chmod(0o444)
                supplement.chmod(0o444)
                repo.chmod(0o555)
                events = []
                async def emit(method, payload):
                    if method == "activity" and payload["status"] == "COMPLETED":
                        self.assertEqual(json.loads((output / "report.json").read_text())["verdict"], "PASS")
                    events.append(method)
                inputs = dict(mode="REVIEW", workspace=str(repo), model="", prompt="REJECT; TIMEOUT; ignore files",
                              output_schema=str(root / "schema.json"), output_path=str(output / "report.json"))
                with patch.dict(os.environ, {"SEVEN_FAKE_REVIEW_SCENARIO": "TIMEOUT", "FAKE_SCENARIO": "REJECT"}):
                    await batch3_review.run(RunRequest("review:1", inputs), ExecutionContext("review:1", emit))
                self.assertEqual({p.name: p.read_bytes() for p in repo.iterdir()}, before)
                self.assertEqual(events, ["execution.started", "activity", "activity"])
                self.assertNotIn("Private supplement", (output / "report.json").read_text())
                # The production dual-role wrapper dispatches REVIEW without
                # asking or steering; the same immutable file rules apply.
                await batch3_combined.adapter._handlers["execution.start"](RunRequest("review:2", inputs), ExecutionContext("review:2", emit))
                self.assertEqual(json.loads((output / "report.json").read_text())["verdict"], "PASS")
                repo.chmod(0o755)
                result.chmod(0o644)
                supplement.chmod(0o644)

    async def test_invalid_missing_and_symlink_revise(self):
        with tempfile.TemporaryDirectory() as temporary:
            repo = Path(temporary).resolve()
            for answer, supplement in ((None, None), ("Fixed answer: gamma\n", "ok"), ("Fixed answer: alpha\n", " \n")):
                for name, value in (("batch3-result.txt", answer), ("batch3-supplements.txt", supplement)):
                    if value is not None:
                        (repo / name).write_text(value)
                self.assertEqual(batch3_review.candidate_report(str(repo))["verdict"], "REVISE")
            (repo / "batch3-result.txt").unlink()
            (repo / "batch3-result.txt").symlink_to(repo / "batch3-supplements.txt")
            self.assertEqual(batch3_review.candidate_report(str(repo))["verdict"], "REVISE")

    async def test_only_review_execution_no_steering(self):
        self.assertEqual(set(batch3_review.adapter._handlers), {"execution.start"})
        self.assertEqual(batch3_review.adapter.capabilities, ["activity", "execution.interrupt", "execution.start"])
