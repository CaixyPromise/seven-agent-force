import asyncio
import importlib.util
import json
import os
from pathlib import Path
import sys
import tempfile
import unittest
from unittest.mock import patch

ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(ROOT))
sys.path.insert(0, str(ROOT.parents[1] / "runtime-sdk" / "src"))
spec = importlib.util.spec_from_file_location("codex_adapter", ROOT / "adapter.py")
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)
from seven_agent.force.agent import ExecutionContext, RunRequest, ResumeRequest, RoundOutcome
from seven_agent.force.interaction import InteractionAnswer, UserMessage
from seven_agent.force.tools import ToolResult


class AdapterTests(unittest.IsolatedAsyncioTestCase):
    async def asyncSetUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name)
        self.inputs = dict(mode="AGENT", taskId="9223372036854775807", attemptId="123",
                           instruction="normal", workspace=str(self.root), output=str(self.root / "output"),
                           state=str(self.root / "state"), temporary=str(self.root / "tmp"),
                           provider={"model": "frozen-model"}, feedback="", reviewCorrection=None,
                           availableTools=[{
                               "name": module.app_server.NETWORK_TOOL_NAME,
                               "description": module.app_server.NETWORK_TOOL_DESCRIPTION,
                               "inputSchema": module.app_server.NETWORK_TOOL.input_schema,
                           }])
        self.events = []
        async def emit(method, payload):
            self.events.append((method, payload))
        capabilities = module.adapter.capabilities
        self.context = ExecutionContext("round-1", emit, capabilities=capabilities)
        native = module.app_server.asyncio.create_subprocess_exec
        async def spawn(*args, **kwargs):
            self.assertEqual(args[0], "/usr/local/bin/codex")
            return await native(sys.executable, str(ROOT / "tests/mock_codex.py"), *args[1:], **kwargs)
        self.spawn = patch.object(module.app_server.asyncio, "create_subprocess_exec", side_effect=spawn)
        self.spawn.start()
        self.addCleanup(self.spawn.stop)
        module.runtime = module.CodexAppServer(safe_text=module.safe_assistant_text)

    async def asyncTearDown(self):
        await module.runtime.shutdown()

    async def test_start_activity_session_and_fixed_provider(self):
        with patch.dict(os.environ, {"EXTERNAL_SECRET": "secret", "HTTP_PROXY": "external"}, clear=True):
            result = await module.run(RunRequest("round-1", self.inputs), self.context)
        self.assertEqual(result.outcome, RoundOutcome.FINISHED)
        self.assertEqual(result.session_reference, "native-session-123")
        invocation = json.loads((self.root / "invocation.json").read_text())
        self.assertEqual(invocation["args"][:2], ["app-server", "--stdio"])
        self.assertIn("features.default_mode_request_user_input=true", invocation["args"])
        self.assertNotIn("EXTERNAL_SECRET", invocation["env"])
        self.assertEqual(invocation["env"]["OPENAI_API_KEY"], "seven-local-provider-facade")
        activities = [value[1] for value in self.events if value[0] == "activity"]
        self.assertEqual([value["status"] for value in activities], ["STARTED", "SUCCEEDED"])
        catalogs = [value[1] for value in self.events if value[0] == "tool.catalog"]
        self.assertEqual(catalogs[0]["toolNames"], [module.app_server.NETWORK_TOOL_NAME])
        self.assertRegex(catalogs[0]["catalogDigest"], r"^[0-9a-f]{64}$")
        self.assertIn("Public answer", json.dumps(self.events))

        native = json.loads((self.root / "thread.json").read_text())
        self.assertEqual(native["sandbox"], "danger-full-access")
        initialized = json.loads((self.root / "initialize.json").read_text())
        self.assertIs(initialized["capabilities"]["experimentalApi"], True)
        tools = native["dynamicTools"]
        self.assertEqual(len(tools), 1)
        self.assertEqual(tools[0]["name"], module.app_server.NETWORK_TOOL_NAME)
        self.assertEqual(tools[0]["description"], module.app_server.NETWORK_TOOL_DESCRIPTION)
        self.assertIn("cannot reach a required external host", tools[0]["description"])
        self.assertIn("do not retry the command through the normal shell while this call is pending", tools[0]["description"])
        native_turn = json.loads((self.root / "turn-start.json").read_text())
        native_prompt = native_turn["input"][0]["text"]
        self.assertIn("Native platform tools registered for this turn", native_prompt)
        self.assertIn(module.app_server.NETWORK_TOOL_NAME, native_prompt)
        self.assertIn("do not search for an executable or shell command", native_prompt)
        # Keep the image-owned definition byte-for-byte compatible with the
        # Go Supervisor definition. A missing JSON Schema keyword makes the
        # strict frozen-tool handshake fail before Codex starts.
        self.assertEqual(
            tools[0]["inputSchema"]["properties"]["targets"]["items"]["properties"]["protocol"],
            {"type": "string", "enum": ["http", "https"]},
        )

    def test_available_tool_is_required_and_must_match_frozen_definition(self):
        missing = dict(self.inputs)
        del missing["availableTools"]
        with self.assertRaisesRegex(module.AdapterFailure, "CODEX_INPUT_INVALID"):
            module.prepare(missing)

        for changed in (
                [{**self.inputs["availableTools"][0], "name": "other_tool"}],
                [{**self.inputs["availableTools"][0], "description": "changed"}],
                [{**self.inputs["availableTools"][0], "inputSchema": {
                    **self.inputs["availableTools"][0]["inputSchema"], "required": []}}],
                [self.inputs["availableTools"][0], self.inputs["availableTools"][0]]):
            with self.assertRaisesRegex(module.AdapterFailure,
                                        "CODEX_DYNAMIC_TOOL_UNAVAILABLE"):
                module.prepare(dict(self.inputs, availableTools=changed))

        both = self.inputs["availableTools"] + [{
            "name": module.app_server.GIT_DELIVERY_TOOL_NAME,
            "description": module.app_server.GIT_DELIVERY_TOOL_DESCRIPTION,
            "inputSchema": module.app_server.GIT_DELIVERY_TOOL.input_schema,
        }]
        prepared = module.prepare(dict(self.inputs, availableTools=both))
        self.assertEqual([tool.name for tool in prepared[-1]], [
            module.app_server.NETWORK_TOOL_NAME,
            module.app_server.GIT_DELIVERY_TOOL_NAME,
        ])

    async def test_resume_re_registers_frozen_dynamic_tool(self):
        first = await module.run(RunRequest("round-1", self.inputs), self.context)
        await module.runtime.shutdown()
        module.runtime = module.CodexAppServer(safe_text=module.safe_assistant_text)
        context = ExecutionContext("round-2", lambda *_: asyncio.sleep(0),
                                   capabilities=module.adapter.capabilities)
        await module.resume(ResumeRequest("round-2", self.inputs,
                                          first.session_reference), context)
        native = json.loads((self.root / "thread.json").read_text())
        self.assertEqual(native["dynamicTools"],
                         [module.app_server.NETWORK_TOOL.wire()])

    async def test_frozen_allowlist_registers_git_delivery_tool(self):
        self.inputs["availableTools"] = [{
            "name": module.app_server.GIT_DELIVERY_TOOL_NAME,
            "description": module.app_server.GIT_DELIVERY_TOOL_DESCRIPTION,
            "inputSchema": module.app_server.GIT_DELIVERY_TOOL.input_schema,
        }]
        self.inputs["instruction"] = "CALL_GIT_DELIVERY_TOOL"
        task = asyncio.create_task(module.run(RunRequest("round-1", self.inputs), self.context))
        for _ in range(100):
            requests = [payload for method, payload in self.events if method == "tool.request"]
            if requests:
                break
            await asyncio.sleep(.01)
        request = requests[0]
        self.assertEqual(request["toolName"], module.app_server.GIT_DELIVERY_TOOL_NAME)
        self.assertEqual(set(request["arguments"]), {"summary"})
        pending = self.context._tool_requests[request["localToolCallKey"]][1]
        result = ToolResult(request["nativeToolCallId"], request["localToolCallKey"],
                            "103", "SCHEDULED", None,
                            "Git delivery intent saved and scheduled after required gates.",
                            "", "a" * 64, "")
        self.context._tool_results[request["localToolCallKey"]] = result
        pending.set_result(result)
        await asyncio.wait_for(task, 3)
        native = json.loads((self.root / "thread.json").read_text())
        self.assertEqual(native["dynamicTools"], [module.app_server.GIT_DELIVERY_TOOL.wire()])
        delivered = json.loads((self.root / "tool-result.json").read_text())
        self.assertIs(delivered["result"]["success"], True)
        self.assertEqual(json.loads(delivered["result"]["contentItems"][0]["text"])["status"],
                         "SCHEDULED")

    async def test_unlisted_dynamic_tool_is_rejected_without_platform_request(self):
        self.inputs["availableTools"] = []
        self.inputs["instruction"] = "CALL_GIT_DELIVERY_TOOL"
        await asyncio.wait_for(module.run(RunRequest("round-1", self.inputs), self.context), 3)
        self.assertFalse(any(method == "tool.request" for method, _ in self.events))

    async def test_resume_uses_thread_resume_and_restored_answer_context(self):
        first = await module.run(RunRequest("round-1", self.inputs), self.context)
        await module.runtime.shutdown()
        module.runtime = module.CodexAppServer(safe_text=module.safe_assistant_text)
        question = {"interactionKey": "saved", "isBlocking": True, "questions": [{"key": "scope", "title": "Scope", "description": "", "kind": "TEXT", "required": True, "options": [], "allowFreeText": True}]}
        response = {"interactionKey": "saved", "responseId": "101", "answers": [{"questionKey": "scope", "selectedOptionKeys": [], "text": "small"}]}
        values = dict(self.inputs, feedback="continue", interactionQuestionRequest=question, interactionResponse=response)
        context = ExecutionContext("round-2", lambda *_: asyncio.sleep(0), capabilities=module.adapter.capabilities)
        result = await module.resume(ResumeRequest("round-2", values, first.session_reference), context)
        self.assertEqual(result.session_reference, first.session_reference)

    async def test_cold_resume_uses_current_attempt_rollout_without_index_rewrite(self):
        first = await module.run(RunRequest("round-1", self.inputs), self.context)
        old_state = Path(self.inputs["state"])
        restored_state = self.root / "next-attempt" / "state"
        codex_home = restored_state / "codex"
        rollout = codex_home / "sessions" / "2026" / "10" / "05" / (
            "rollout-2026-10-05T00-00-00-" + first.session_reference + ".jsonl")
        rollout.parent.mkdir(parents=True)
        rollout.write_text("restored native history")
        index = codex_home / "state_5.sqlite"
        index.write_bytes(b"immutable fixture with obsolete prior-attempt path")
        module.runtime = module.CodexAppServer(safe_text=module.safe_assistant_text)
        context = ExecutionContext("round-2", lambda *_: asyncio.sleep(0),
                                   capabilities=module.adapter.capabilities)
        values = dict(self.inputs, state=str(restored_state), feedback="continue")
        result = await module.resume(ResumeRequest("round-2", values, first.session_reference), context)
        native = json.loads((self.root / "thread.json").read_text())
        self.assertEqual(native["path"], str(rollout.resolve()))
        self.assertEqual(native["threadId"], first.session_reference)
        self.assertNotIn(str(old_state), native["path"])
        self.assertEqual(result.session_reference, first.session_reference)
        self.assertEqual(index.read_bytes(), b"immutable fixture with obsolete prior-attempt path")

    def test_restored_rollout_rejects_ambiguous_and_outside_state_files(self):
        home = self.root / "codex"
        sessions = home / "sessions"
        sessions.mkdir(parents=True)
        first = sessions / "rollout-first-session-1.jsonl"
        first.write_text("one")
        second = sessions / "rollout-second-session-1.jsonl"
        second.write_text("two")
        with self.assertRaisesRegex(module.AppServerFailure, "CODEX_SESSION_ROLLOUT_INVALID"):
            module.app_server._restored_rollout_path(home, "session-1")
        outside = self.root / "outside.jsonl"
        outside.write_text("outside")
        (sessions / "rollout-first-session-2.jsonl").symlink_to(outside)
        with self.assertRaisesRegex(module.AppServerFailure, "CODEX_SESSION_ROLLOUT_INVALID"):
            module.app_server._restored_rollout_path(home, "session-2")

    async def test_resume_identity_mismatch_does_not_start_continuation(self):
        command, env, cwd, *_ = module.prepare(self.inputs)
        await module.runtime._ensure(command, env, cwd)
        async def wrong_response(method, params):
            self.assertEqual(method, "thread/resume")
            return {"thread": {"id": "wrong-session"}}
        with patch.object(module.app_server.AppServerClient, "request", side_effect=wrong_response):
            with self.assertRaisesRegex(module.AdapterFailure, "CODEX_SESSION_IDENTITY_MISMATCH"):
                await module.resume(ResumeRequest("round-2", self.inputs, "expected-session"), self.context)
        self.assertFalse((self.root / "turn-start.json").exists())

    async def test_resume_injects_operation_result_before_continuation(self):
        first = await module.run(RunRequest("round-1", self.inputs), self.context)
        result_value = ToolResult("native-call-1", "platform-tool-1", "101", "SUCCEEDED", 0,
                                  "Installed", "bounded output", "a" * 64, "")
        values = dict(self.inputs, feedback="MUST_NOT_APPEAR_IN_NATIVE_INPUT",
                      operationToolResult=result_value.wire())
        context = ExecutionContext("round-2", lambda *_: asyncio.sleep(0),
                                   capabilities=module.adapter.capabilities)

        result = await module.resume(
            ResumeRequest("round-2", values, first.session_reference), context)

        self.assertEqual(result.session_reference, first.session_reference)
        injected = json.loads((self.root / "inject-items.json").read_text())
        self.assertEqual(set(injected), {"threadId", "items"})
        self.assertEqual(injected["threadId"], first.session_reference)
        item = injected["items"][0]
        self.assertEqual(set(item), {"type", "call_id", "name", "output"})
        self.assertEqual(item["type"], "custom_tool_call_output")
        self.assertEqual(item["call_id"], "native-call-1")
        self.assertNotEqual(item["call_id"], result_value.operation_id)
        self.assertEqual(item["name"], module.app_server.NETWORK_TOOL_NAME)
        self.assertEqual(item["output"][0]["type"], "input_text")
        restored = json.loads(item["output"][0]["text"])
        self.assertEqual(restored, result_value.wire())
        native_turn = json.loads((self.root / "turn-start.json").read_text())
        self.assertEqual(native_turn, {
            "threadId": first.session_reference,
            "input": [{"type": "text", "text": module.app_server.OPERATION_CONTINUATION_TEXT}],
        })
        serialized_turn = json.dumps(native_turn)
        self.assertNotIn("MUST_NOT_APPEAR_IN_NATIVE_INPUT", serialized_turn)
        self.assertNotIn("bounded output", serialized_turn)
        self.assertNotIn("toolOutput", serialized_turn)

    def test_operation_tool_result_is_strictly_resume_only(self):
        valid = ToolResult("native-call-1", "platform-tool-1", "101", "DENIED", None, "", "", "",
                           "USER_DECLINED_NETWORK_ACCESS").wire()
        with self.assertRaisesRegex(module.AdapterFailure, "CODEX_OPERATION_RESTORE_INVALID"):
            module.prepare(dict(self.inputs, operationToolResult=valid))
        with self.assertRaisesRegex(module.AdapterFailure, "CODEX_OPERATION_RESTORE_INVALID"):
            module.prepare(dict(self.inputs, operationToolResult={**valid, "requestId": "json-rpc-id"}),
                           "native-session-123")
        for change in ({"nativeToolCallId": ""}, {"operationId": 101},
                       {"resultDigest": "b" * 63}):
            with self.assertRaisesRegex(module.AdapterFailure, "CODEX_OPERATION_RESTORE_INVALID"):
                module.prepare(dict(self.inputs,
                                    operationToolResult={**valid, **change}),
                               "native-session-123")
        question = {"interactionKey": "saved", "isBlocking": True, "questions": []}
        response = {"interactionKey": "saved", "responseId": "101", "answers": []}
        with self.assertRaisesRegex(module.AdapterFailure, "CODEX_OPERATION_RESTORE_INVALID"):
            module.prepare(dict(self.inputs, operationToolResult=valid,
                                interactionQuestionRequest=question,
                                interactionResponse=response), "native-session-123")

    async def test_injection_response_loss_fails_closed_before_continuation(self):
        first = await module.run(RunRequest("round-1", self.inputs), self.context)
        (self.root / "turn-start.json").unlink()
        result_value = ToolResult("native-call-response-loss", "platform-tool-loss", "102",
                                  "SUCCEEDED", 0, "Installed", "bounded", "b" * 64, "")
        values = dict(self.inputs, feedback="must not run",
                      operationToolResult=result_value.wire())
        context = ExecutionContext("round-loss", lambda *_: asyncio.sleep(0),
                                   capabilities=module.adapter.capabilities)

        with self.assertRaisesRegex(module.AdapterFailure,
                                    "CODEX_OPERATION_INJECTION_OUTCOME_UNKNOWN"):
            await module.resume(ResumeRequest(
                "round-loss", values, first.session_reference), context)

        self.assertTrue((self.root / "inject-items.json").exists())
        self.assertFalse((self.root / "turn-start.json").exists())

    async def test_fixed_mock_rejects_turn_start_tool_output(self):
        command, env, cwd, *_ = module.prepare(self.inputs)
        client = module.app_server.AppServerClient(
            command, env, cwd, lambda *_: asyncio.sleep(0),
            lambda *_: asyncio.sleep(0))
        await client.start()
        try:
            with self.assertRaisesRegex(module.AppServerFailure,
                                        "CODEX_APP_SERVER_REQUEST_FAILED"):
                await client.request("turn/start", {
                    "threadId": "native-session-123", "input": [],
                    "toolOutput": {"name": module.app_server.NETWORK_TOOL_NAME,
                                   "namespace": None, "output": "{}"},
                })
        finally:
            await client.close()
        rejected = json.loads((self.root / "rejected-turn-start.json").read_text())
        self.assertIn("toolOutput", rejected)

    async def test_native_blocking_question_answer_and_no_duplicate_start(self):
        self.inputs["instruction"] = "ASK_NATIVE_QUESTION"
        task = asyncio.create_task(module.run(RunRequest("round-1", self.inputs), self.context))
        for _ in range(100):
            if any(method == "interaction.request" for method, _ in self.events):
                break
            await asyncio.sleep(.01)
        request = next(payload for method, payload in self.events if method == "interaction.request")
        self.assertTrue(request["isBlocking"])
        self.assertEqual(request["questions"][0]["options"][0]["label"], "Small")
        pending = self.context._questions[request["interactionKey"]][1]
        pending.set_result((InteractionAnswer("scope", ("o1-1",), ""),))
        result = await asyncio.wait_for(task, 3)
        self.assertEqual(result.outcome, RoundOutcome.FINISHED)
        self.assertIn("Continued after answer", json.dumps(self.events))

    def test_native_questions_opt_in_is_agent_only(self):
        schema = self.root / "schema.json"
        schema.write_text('{}')
        command, *_ = module.prepare(dict(mode="REVIEW", workspace=str(self.root),
                                         model="frozen-model", prompt="Review only",
                                         output_schema=str(schema),
                                         output_path=str(self.root / "review.json")))
        self.assertNotIn("features.default_mode_request_user_input=true", command)

    async def test_native_default_mode_question_uses_existing_answer_bridge(self):
        self.inputs["instruction"] = "ASK_NATIVE_DEFAULT"
        task = asyncio.create_task(module.run(RunRequest("round-1", self.inputs), self.context))
        for _ in range(100):
            requests = [payload for method, payload in self.events if method == "interaction.request"]
            if requests:
                break
            await asyncio.sleep(.01)
        self.assertTrue(requests, "Default-mode opt-in must reach the native question bridge")
        request = requests[0]
        # Codex 0.147.0 marks Default-mode questions as nonblocking. Preserve
        # that metadata instead of fabricating Plan mode or a blocking card.
        self.assertFalse(request["isBlocking"])
        self.assertFalse(task.done())
        self.context._questions[request["interactionKey"]][1].set_result(
            (InteractionAnswer("scope", ("o1-1",), ""),))
        result = await asyncio.wait_for(task, 3)
        self.assertEqual(result.outcome, RoundOutcome.FINISHED)
        native = json.loads((self.root / "question-result.json").read_text())
        self.assertEqual(native["result"]["answers"]["scope"]["answers"], ["Small"])

    async def test_dynamic_network_tool_hot_path_and_native_receipt(self):
        self.inputs["instruction"] = "CALL_NETWORK_TOOL"
        task = asyncio.create_task(module.run(RunRequest("round-1", self.inputs), self.context))
        for _ in range(100):
            requests = [payload for method, payload in self.events if method == "tool.request"]
            if requests:
                break
            await asyncio.sleep(.01)
        request = requests[0]
        self.assertEqual(request["toolName"], module.app_server.NETWORK_TOOL_NAME)
        self.assertEqual(request["arguments"]["targets"], [
            {"host": "registry.npmjs.org", "protocol": "https", "port": 443}])
        pending = self.context._tool_requests[request["localToolCallKey"]][1]
        self.assertEqual(request["nativeToolCallId"], "tool-call-1")
        tool_result = ToolResult(request["nativeToolCallId"], request["localToolCallKey"],
                                 "101", "SUCCEEDED", 0,
                                 "Installed", "ok", "a" * 64, "")
        self.context._tool_results[request["localToolCallKey"]] = tool_result
        pending.set_result(tool_result)
        result = await asyncio.wait_for(task, 3)
        self.assertEqual(result.outcome, RoundOutcome.FINISHED)
        receipt = next(payload for method, payload in self.events if method == "tool.received")
        self.assertEqual((receipt["operationId"], receipt["status"]), ("101", "RECEIVED"))
        native = json.loads((self.root / "tool-result.json").read_text())
        self.assertIs(native["result"]["success"], True)
        self.assertEqual(native["result"]["contentItems"][0]["type"], "inputText")
        content = json.loads(native["result"]["contentItems"][0]["text"])
        self.assertEqual((content["operationId"], content["status"]), ("101", "SUCCEEDED"))

    def test_dynamic_network_tool_normalizes_native_absolute_workspace_cwd(self):
        request = module.app_server._tool_request("tool-call-1", module.app_server.NETWORK_TOOL_NAME, {
            "purpose": "Install dependencies",
            "command": "pnpm install --frozen-lockfile",
            "cwd": str(self.root / "packages"),
            "targets": [{"host": "registry.npmjs.org", "protocol": "https", "port": 443}],
        }, self.root)
        self.assertEqual(request.arguments["cwd"], "packages")

    def test_dynamic_network_tool_rejects_native_absolute_cwd_outside_workspace(self):
        with self.assertRaisesRegex(module.app_server.AppServerFailure,
                                    "CODEX_DYNAMIC_TOOL_INPUT_INVALID"):
            module.app_server._tool_request("tool-call-1", module.app_server.NETWORK_TOOL_NAME, {
                "purpose": "Install dependencies",
                "command": "pnpm install --frozen-lockfile",
                "cwd": "/tmp/outside-workspace",
                "targets": [{"host": "registry.npmjs.org", "protocol": "https", "port": 443}],
            }, self.root)

    async def test_native_command_approval_remains_fail_closed(self):
        self.inputs["instruction"] = "REQUEST_NATIVE_APPROVAL"
        result = await asyncio.wait_for(module.run(RunRequest("round-1", self.inputs), self.context), 3)
        self.assertEqual(result.outcome, RoundOutcome.FINISHED)
        native = json.loads((self.root / "approval-result.json").read_text())
        self.assertEqual(native["error"]["code"], -32001)
        self.assertFalse(any(method == "tool.request" for method, _ in self.events))

    async def test_turn_steer_is_bound_to_active_turn(self):
        self.inputs["instruction"] = "ASK_NATIVE_QUESTION"
        task = asyncio.create_task(module.run(RunRequest("round-1", self.inputs), self.context))
        for _ in range(100):
            if module.runtime.turn_id:
                break
            await asyncio.sleep(.01)
        status = await module.message(UserMessage("round-1", "message-1", "supplement"), self.context)
        self.assertEqual(status.value, "RECEIVED")
        for pending in self.context._questions.values():
            if not pending[1].done():
                pending[1].set_result((InteractionAnswer("scope", (), "custom"),))
        await asyncio.wait_for(task, 3)

    async def test_native_nonblocking_question_does_not_hold_turn_open(self):
        self.inputs["instruction"] = "ASK_NATIVE_NONBLOCKING"
        result = await asyncio.wait_for(module.run(RunRequest("round-1", self.inputs), self.context), 3)
        self.assertEqual(result.outcome, RoundOutcome.FINISHED)
        request = next(payload for method, payload in self.events if method == "interaction.request")
        self.assertFalse(request["isBlocking"])
        closed = next(payload for method, payload in self.events if method == "interaction.closed")
        self.assertEqual((closed["interactionKey"], closed["reason"]), (request["interactionKey"], "TURN_COMPLETED"))

    async def test_native_resolved_notification_closes_or_suppresses_question(self):
        self.inputs["instruction"] = "NATIVE_REQUEST_RESOLVED"
        result = await asyncio.wait_for(module.run(RunRequest("round-1", self.inputs), self.context), 3)
        self.assertEqual(result.outcome, RoundOutcome.FINISHED)
        requests = [payload for method, payload in self.events if method == "interaction.request"]
        closed = [payload for method, payload in self.events if method == "interaction.closed"]
        self.assertLessEqual(len(requests), 1)
        if requests:
            self.assertEqual(len(closed), 1)
            self.assertEqual(closed[0]["reason"], "NATIVE_REQUEST_CLEARED")

    async def test_secret_native_question_is_not_exposed(self):
        self.inputs["instruction"] = "ASK_NATIVE_SECRET"
        result = await asyncio.wait_for(module.run(RunRequest("round-1", self.inputs), self.context), 3)
        self.assertEqual(result.outcome, RoundOutcome.FINISHED)
        self.assertFalse(any(method == "interaction.request" for method, _ in self.events))

    async def test_review_uses_output_schema_and_writes_machine_report(self):
        schema = self.root / "schema.json"
        schema.write_text('{"type":"object"}')
        inputs = dict(mode="REVIEW", workspace=str(self.root), model="frozen-model", prompt="review",
                      output_schema=str(schema), output_path=str(self.root / "report.json"))
        context = ExecutionContext("review-1", lambda *_: asyncio.sleep(0), capabilities=module.adapter.capabilities)
        result = await module.run(RunRequest("review-1", inputs), context)
        self.assertEqual(result.outcome, RoundOutcome.FINISHED)
        report = json.loads((self.root / "report.json").read_text())
        self.assertEqual(report["verdict"], "PASS")
        native = json.loads((self.root / "thread.json").read_text())
        self.assertEqual(native["sandbox"], "danger-full-access")

    def test_review_report_creation_rejects_symlink_race(self):
        destination = self.root / "report.json"
        outside = self.root / "outside.json"
        destination.symlink_to(outside)
        with self.assertRaises(module.AdapterFailure):
            module._write_review_report(destination, '{"verdict":"PASS"}')
        self.assertFalse(outside.exists())

    async def test_shutdown_reaps_app_server_child(self):
        await module.run(RunRequest("round-1", self.inputs), self.context)
        # A native turn is quiesced before Processor/Checkpoint runs.  The
        # next round resumes the same thread in a fresh App Server process.
        self.assertIsNone(module.runtime.client)
        await module.runtime.shutdown()
        self.assertIsNone(module.runtime.client)

    def test_closed_inputs_and_app_server_command(self):
        command, env, _, _, model, provider, _, operation_tool_result, frozen_tools = module.prepare(self.inputs)
        self.assertEqual(command[:3], ["/usr/local/bin/codex", "app-server", "--stdio"])
        self.assertNotIn("exec", command)
        self.assertEqual((model, provider), ("frozen-model", "seven"))
        self.assertIsNone(operation_tool_result)
        self.assertEqual([tool.wire() for tool in frozen_tools],
                         [module.app_server.NETWORK_TOOL.wire()])
        self.assertIn('model_providers.seven.base_url="http://127.0.0.1:8787/v1"', command)
        for extra in ("program", "env", "provider_url"):
            with self.assertRaises(module.AdapterFailure):
                module.prepare(dict(self.inputs, **{extra: "untrusted"}))


if __name__ == "__main__":
    unittest.main()
