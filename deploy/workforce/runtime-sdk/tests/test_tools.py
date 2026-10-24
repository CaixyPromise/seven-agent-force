import contextlib
import io
import json
from pathlib import Path
import tempfile
import unittest

from seven_agent.force.__main__ import main
from seven_agent.force._protocol import VERSION, encode


class ToolTests(unittest.TestCase):
    def test_inspect_and_build_do_not_start_handler(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            source = root / "adapter.py"
            source.write_text('''from seven_agent.force.agent import AgentAdapter
agent = AgentAdapter(name="test", version="1")
@agent.run
async def run(request, context):
    raise AssertionError("handler must not run during inspection")
''')
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                self.assertEqual(main(["inspect", str(source)]), 0)
            self.assertEqual(json.loads(output.getvalue())["id"], "test")
            output = io.StringIO()
            with contextlib.redirect_stdout(output):
                self.assertEqual(main(["dev", str(source)]), 0)
            self.assertEqual(json.loads(output.getvalue()), {"status":"HANDSHAKE_ONLY", "executions":0})
            with contextlib.redirect_stdout(io.StringIO()):
                self.assertEqual(main(["build", str(source), "--output", str(root / "build"),
                                       "--image-module", "/opt/seven/adapter/main.py"]), 0)
            manifest = json.loads((root / "build/adapter-manifest.json").read_text())
            self.assertEqual(len(manifest["adapter"]["binarySha256"]), 64)
            with contextlib.redirect_stderr(io.StringIO()):
                self.assertEqual(main(["build", str(source), "--output", str(root / "build"),
                                       "--image-module", "/opt/seven/adapter/main.py"]), 1)

    def test_check_fails_nonzero_for_bad_transcript(self):
        with tempfile.TemporaryDirectory() as directory:
            path = Path(directory) / "frames.jsonl"
            path.write_bytes(encode(dict(protocol=VERSION, method="initialize", key="i1", payload={})))
            with contextlib.redirect_stdout(io.StringIO()):
                self.assertEqual(main(["check", str(path), "--direction", "down"]), 0)
            path.write_bytes(b'{"bad":"secret-do-not-echo"}\n')
            error = io.StringIO()
            with contextlib.redirect_stderr(error):
                self.assertEqual(main(["check", str(path), "--direction", "down"]), 1)
            self.assertNotIn("secret-do-not-echo", error.getvalue())

    def test_dev_runs_only_explicit_fixture(self):
        with tempfile.TemporaryDirectory() as directory:
            source=Path(directory)/"adapter.py"
            source.write_text('''from seven_agent.force.agent import AgentAdapter, RoundResult
agent=AgentAdapter(name="fixture",version="1")
@agent.run
async def run(request,context):
    assert request.inputs == {"local": True}
    await context.started()
    await context.emit_message("fixture observed")
    return RoundResult.finished()
''')
            fixture=Path(directory)/"input.json"
            fixture.write_text('{"local":true}')
            output=io.StringIO()
            with contextlib.redirect_stdout(output):
                self.assertEqual(main(["dev",str(source),"--fixture",str(fixture)]),0)
            self.assertEqual(json.loads(output.getvalue())["status"],"LOCAL_FIXTURE_FINISHED")
