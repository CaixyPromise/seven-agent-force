import asyncio
import importlib.util
import json
import os
from pathlib import Path
import stat
import sys
import tempfile
import unittest
from unittest import mock


ROOT = Path(__file__).resolve().parents[1]
SDK = ROOT.parents[1] / "runtime-sdk" / "src"
sys.path.insert(0, str(SDK))
spec = importlib.util.spec_from_file_location("claude_adapter", ROOT / "adapter.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
from seven_agent.force.agent import RunRequest
from seven_agent.force.interaction import DeliveryStatus, UserMessage


class Context:
    def __init__(self):
        self.pid = None
        self.catalog = None
        self.messages = []

    async def started(self, *, native_pid=None):
        self.pid = native_pid

    async def tool_catalog_ready(self, names):
        self.catalog = names

    async def emit_message(self, text):
        self.messages.append(text)


class ClaudeAdapterTests(unittest.IsolatedAsyncioTestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        root = Path(self.temp.name)
        self.root = root
        self.repo = root / "repo"
        self.repo.mkdir()
        self.fake = root / "claude"
        self.fake.write_text("""#!/usr/bin/env python3
import json,sys
request=json.loads(sys.stdin.readline())
print(json.dumps({'type':'system','subtype':'init','session_id':'session_test'}), flush=True)
if '--json-schema' in sys.argv:
 print(json.dumps({'type':'result','session_id':'session_test','structured_output':{'schemaVersion':'review-report/v1','verdict':'PASS','score':100,'summary':'ok','reviewTests':[]}}), flush=True)
else:
 print(json.dumps({'type':'assistant','session_id':'session_test','message':{'content':[{'type':'text','text':'done'}]}}), flush=True)
 print(json.dumps({'type':'result','session_id':'session_test','result':'done'}), flush=True)
""")
        self.fake.chmod(self.fake.stat().st_mode | stat.S_IXUSR)
        module.CLAUDE = str(self.fake)
        module.BRIDGE = str(ROOT / "mcp_bridge.py")

    def tearDown(self):
        self.temp.cleanup()

    def agent_inputs(self):
        paths = {}
        for name in ("output", "state", "temporary"):
            paths[name] = str(self.root / name)
            (self.root / name).mkdir()
        return {"mode": "AGENT", "taskId": "11", "attemptId": "12", "instruction": "fix it",
                "workspace": str(self.repo), **paths, "provider": {"model": "claude-test"},
                "feedback": "", "availableTools": []}

    async def test_agent_uses_primary_model_and_preserves_session(self):
        context = Context()
        result = await module.execute(RunRequest("round-one", self.agent_inputs()), context)
        self.assertEqual(result.session_reference, "session_test")
        self.assertEqual(result.summary, "done")
        self.assertGreater(context.pid, 0)
        self.assertEqual(context.catalog, ())
        self.assertEqual(context.messages, ["done"])
        self.assertFalse(any(self.root.glob("state/mcp-*")))

    async def test_review_writes_structured_report(self):
        schema = self.root / "schema.json"
        output = self.root / "report.json"
        schema.write_text(json.dumps({"type": "object"}))
        inputs = {"mode": "REVIEW", "workspace": str(self.repo), "model": "claude-test",
                  "prompt": "review", "output_schema": str(schema), "output_path": str(output)}
        result = await module.execute(RunRequest("round-review", inputs), Context())
        report = json.loads(output.read_text())
        self.assertEqual(result.session_reference, "session_test")
        self.assertEqual(report["verdict"], "PASS")

    async def test_user_message_is_delivered_to_active_stream(self):
        self.fake.write_text("""#!/usr/bin/env python3
import json,sys
initial=json.loads(sys.stdin.readline())
print(json.dumps({'type':'system','subtype':'init','session_id':'session_message'}), flush=True)
supplement=json.loads(sys.stdin.readline())
text=supplement['message']['content']
print(json.dumps({'type':'result','session_id':'session_message','result':text}), flush=True)
""")
        self.fake.chmod(self.fake.stat().st_mode | stat.S_IXUSR)
        context = Context()
        running = asyncio.create_task(module.execute(RunRequest("round-message", self.agent_inputs()), context))
        for _ in range(100):
            if context.pid:
                break
            await asyncio.sleep(0.01)
        status = await module.message(UserMessage("round-message", "message-one", "additional context"), context)
        result = await asyncio.wait_for(running, 5)
        self.assertEqual(status, DeliveryStatus.RECEIVED)
        self.assertEqual(result.summary, "additional context")
        self.assertEqual(await module.message(UserMessage("round-message", "late", "late"), context),
                         DeliveryStatus.UNKNOWN)

    async def test_project_hooks_are_written_to_claude_settings(self):
        digest = "a" * 64
        environment = {
            "SEVEN_TASK_ID": "11",
            "SEVEN_ATTEMPT_ID": "12",
            "SEVEN_ATTEMPT_GENERATION": "1",
            "SEVEN_CONTROL_SOCKET": "/runtime/s",
            "SEVEN_HOOK_CONFIGURATION_ROOT": "/opt/seven-worker/managed-hooks/" + digest,
            "SEVEN_HOOK_CONFIGURATION_DIGEST": digest,
        }
        with mock.patch.dict(os.environ, environment, clear=False):
            await module.execute(RunRequest("round-hooks", self.agent_inputs()), Context())
        settings = json.loads((self.root / "state" / "config" / "settings.json").read_text())
        self.assertIn("claude.PreToolUse", settings["hooks"]["PreToolUse"][0]["hooks"][0]["command"])
        self.assertIn("claude.PostToolUse", settings["hooks"]["PostToolUse"][0]["hooks"][0]["command"])

    def test_manifest_is_anthropic_production_runtime(self):
        manifest = json.loads((ROOT.parents[1] / "images" / "claude-harness-manifest.json").read_text())
        self.assertEqual(manifest["harness"]["version"], "2.1.273")
        self.assertEqual(manifest["scheduling"]["providerProtocols"], ["ANTHROPIC"])
        self.assertIn("REVIEW_RUN", manifest["executionKinds"])
        self.assertNotIn("apiKey", json.dumps(manifest))

    def test_tool_identity_is_stable_per_native_occurrence(self):
        first = module._tool_identity("17", "request_git_delivery")
        self.assertEqual(first, module._tool_identity("17", "request_git_delivery"))
        self.assertNotEqual(first, module._tool_identity("18", "request_git_delivery"))
        self.assertNotEqual(first, module._tool_identity("17", "run_command_with_network_access"))


if __name__ == "__main__":
    unittest.main()
