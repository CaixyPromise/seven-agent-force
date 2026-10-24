"""Interactive real-stdio conformance: no provider or platform side effects."""
import asyncio
import copy
import sys
import unittest

from seven_agent.force._protocol import VERSION, ProtocolError, decode, encode
from seven_agent.force.agent import AgentAdapter, ExecutionContext, Question, QuestionOption, QuestionRequest
from seven_agent.force.interaction import InteractionAnswer, InteractionResponse, answers_from_wire, validate_answers


PROGRAM = '''
import asyncio
from seven_agent.force.agent import *
agent = AgentAdapter(name="interaction-fixture", version="3", capabilities=("interaction.request",))
@agent.run
async def run(request, ctx):
    await ctx.started()
    await ctx.emit_message("run-once")
    answer = await ctx.ask(QuestionRequest("ask", (Question("q", "Choose", kind="SINGLE_CHOICE", options=(QuestionOption("a", "Alpha"),), allow_free_text=False),)))
    await ctx.emit_message("answer-once")
    # Same ask must return its existing answer without emitting again.
    assert answer == await ctx.ask(QuestionRequest("ask", (Question("q", "Choose", kind="SINGLE_CHOICE", options=(QuestionOption("a", "Alpha"),), allow_free_text=False),)))
    return RoundResult.finished(session_reference="fixture-session")
@agent.resume
async def resume(request, ctx):
    return await run(request, ctx)
@agent.message
async def message(request, ctx):
    await ctx.emit_message("message-once:" + request.message_key)
    if request.text == "block":
        await asyncio.Event().wait()
    if request.text == "fail":
        raise ValueError("PRIVATE_BODY")
    return DeliveryStatus.RECEIVED
@agent.suspend
async def suspend(request, ctx):
    await ctx.emit_message("suspend-once")
    return "fixture-session"
agent.serve()
'''


class PipeTests(unittest.IsolatedAsyncioTestCase):
    async def test_restored_answer_requires_normal_delivery_in_current_round(self):
        program = '''
from seven_agent.force.agent import *
agent = AgentAdapter(name="cold", version="3", capabilities=("interaction.request",))
@agent.run
async def run(request, ctx):
    raise AssertionError("must resume")
@agent.resume
async def resume(request, ctx):
    await ctx.started()
    question = QuestionRequest("ask", (Question("q", "Text"),))
    expected = InteractionResponse("ask", "101", (InteractionAnswer("q", text="saved"),))
    answers = await ctx.receive_restored_answer(question, expected)
    assert answers == expected.answers
    assert await ctx.receive_restored_answer(question, expected) == answers
    await ctx.emit_message("continued-once")
    return RoundResult.finished(session_reference=request.session_reference)
agent.serve()
'''
        await self.start(program)
        await self.send("execution.resume", "resume", {"roundKey": "new-round", "inputs": {}, "sessionReference": "old-session"})
        await self.until("execution.started")
        payload = self.answer(roundKey="new-round", answers=[{"questionKey": "q", "selectedOptionKeys": [], "text": "saved"}])
        for name, changes, code in (
            ("old-round", {"roundKey": "old-round"}, "ADAPTER_ROUND_NOT_RUNNING"),
            ("wrong-id", {"responseId": "102"}, "ADAPTER_IDEMPOTENCY_CONFLICT"),
            ("wrong-answer", {"answers": [{"questionKey": "q", "selectedOptionKeys": [], "text": "changed"}]}, "ADAPTER_IDEMPOTENCY_CONFLICT"),
        ):
            await self.send("interaction.respond", name, dict(payload, **changes))
            self.assertEqual((await self.until("error", name))["payload"]["code"], code)
        self.assertFalse(any(f["method"] in {"interaction.request", "interaction.received", "execution.finished"} for f in self.frames))
        await self.send("interaction.respond", "cold-answer", payload)
        receipt = await self.until("interaction.received", "cold-answer")
        self.assertEqual(receipt["payload"], {"roundKey": "new-round", "interactionKey": "ask", "responseId": "101", "status": "RECEIVED"})
        finished = await self.until("execution.finished")
        self.assertEqual(finished["payload"]["sessionReference"], "old-session")
        await self.send("interaction.respond", "cold-retry", payload)
        await self.until("interaction.received", "cold-retry")
        self.assertFalse(any(f["method"] == "interaction.request" for f in self.frames))
        self.assertEqual(sum(f["payload"].get("text") == "continued-once" for f in self.frames), 1)

    async def start(self, program=PROGRAM):
        self.frames = []
        self.process = await asyncio.create_subprocess_exec(
            sys.executable, "-B", "-c", program, stdin=asyncio.subprocess.PIPE,
            stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE)
        self.addAsyncCleanup(self.close)
        await self.send("initialize", "init", {})
        return await self.until("ready")

    async def close(self):
        if self.process.returncode is None:
            self.process.stdin.close()
            try:
                await asyncio.wait_for(self.process.wait(), 3)
            except asyncio.TimeoutError:
                self.process.kill()
                await self.process.wait()
                self.fail("stdio process did not stop on EOF")
        self.assertEqual(await self.process.stderr.read(), b"")
        self.assertEqual(self.process.returncode, 0)

    async def send(self, method, key, payload):
        self.process.stdin.write(encode(dict(protocol=VERSION, method=method, key=key, payload=payload)))
        await self.process.stdin.drain()

    async def until(self, method, reply=None):
        while True:
            raw = await asyncio.wait_for(self.process.stdout.readline(), 3)
            self.assertTrue(raw, "unexpected EOF")
            frame = decode(raw, direction="up")
            self.frames.append(frame)
            if frame["method"] == method and (reply is None or frame.get("replyTo") == reply):
                return frame

    async def ask(self):
        await self.send("execution.start", "start", {"roundKey": "r", "inputs": {}})
        request = await self.until("interaction.request")
        self.assertEqual(request["replyTo"], "start")
        self.assertEqual(next(f for f in self.frames if f["method"] == "execution.started")["replyTo"], "start")

    def answer(self, **changes):
        return {"roundKey": "r", "interactionKey": "ask", "responseId": "101", "answers": [
            {"questionKey": "q", "selectedOptionKeys": ["a"], "text": ""}], **changes}

    async def test_answer_replay_conflict_and_no_reexecution(self):
        ready = await self.start()
        self.assertIn("interaction.request", ready["payload"]["capabilities"])
        await self.ask()
        await self.send("interaction.respond", "unknown", self.answer(interactionKey="unknown"))
        self.assertEqual((await self.until("error"))["payload"]["code"], "ADAPTER_INTERACTION_UNKNOWN")
        invalid = self.answer(answers=[{"questionKey": "q", "selectedOptionKeys": ["bad"], "text": ""}])
        await self.send("interaction.respond", "invalid", invalid)
        self.assertEqual((await self.until("error"))["payload"]["code"], "ADAPTER_ANSWER_INVALID")
        await self.send("interaction.respond", "answer", self.answer())
        self.assertEqual((await self.until("interaction.received"))["payload"]["status"], "RECEIVED")
        await self.until("execution.finished")
        for retry in ("answer", "retry"):
            await self.send("interaction.respond", retry, self.answer())
            await self.until("interaction.received", retry)
        await self.send("interaction.respond", "new-response", self.answer(responseId="102"))
        self.assertEqual((await self.until("error"))["payload"]["code"], "ADAPTER_IDEMPOTENCY_CONFLICT")
        await self.send("interaction.respond", "conflict", invalid)
        self.assertEqual((await self.until("error"))["payload"]["code"], "ADAPTER_IDEMPOTENCY_CONFLICT")
        await self.send("execution.start", "repeat", {"roundKey": "r", "inputs": {}})
        await self.until("execution.finished", "repeat")
        self.assertEqual(sum(f["method"] == "interaction.request" for f in self.frames), 1)
        for text in ("run-once", "answer-once"):
            self.assertEqual(sum(f["payload"].get("text") == text for f in self.frames), 1)

    async def test_interrupt_while_ask_and_message_handler_blocked(self):
        await self.start()
        await self.ask()
        await self.send("user.message", "msg", {"roundKey": "r", "messageKey": "m", "text": "block"})
        await self.until("message")
        await self.send("user.message", "msg-retry", {"roundKey": "r", "messageKey": "m", "text": "block"})
        await self.send("execution.interrupt", "stop", {"roundKey": "r"})
        await self.until("control.ack", "stop")
        finished = await self.until("execution.finished")
        self.assertEqual(finished["payload"]["outcome"], "CANCELLED")
        await self.send("interaction.respond", "late", self.answer())
        self.assertEqual((await self.until("error", "late"))["payload"]["code"], "ADAPTER_ROUND_NOT_RUNNING")
        self.assertFalse(any(f["payload"].get("text") == "answer-once" for f in self.frames))
        self.assertEqual(sum(f["payload"].get("text") == "message-once:m" for f in self.frames), 1)

    async def test_message_receipts_duplicate_and_exception_redaction(self):
        await self.start()
        await self.ask()
        for body, status in (("hello", "RECEIVED"), ("fail", "UNKNOWN")):
            payload = {"roundKey": "r", "messageKey": body, "text": body}
            for key in (body, body + "-retry"):
                await self.send("user.message", key, payload)
                self.assertEqual((await self.until("message.received", key))["payload"]["status"], status)
            self.assertEqual(sum(f["payload"].get("text") == "message-once:" + body for f in self.frames), 1)
        self.assertNotIn("PRIVATE_BODY", repr(self.frames))

    async def test_suspend_is_terminal_replayable_and_resume_keeps_session(self):
        await self.start()
        await self.ask()
        await self.send("execution.suspend", "suspend", {"roundKey": "r"})
        result = await self.until("execution.suspended", "suspend")
        self.assertEqual(result["payload"], {"roundKey": "r", "sessionReference": "fixture-session"})
        await self.send("execution.suspend", "again", {"roundKey": "r"})
        await self.until("execution.suspended", "again")
        await self.send("execution.start", "repeat", {"roundKey": "r", "inputs": {}})
        await self.until("execution.suspended", "repeat")
        self.assertFalse(any(f["method"] == "execution.finished" for f in self.frames))
        self.assertEqual(sum(f["payload"].get("text") == "suspend-once" for f in self.frames), 1)
        await self.send("execution.resume", "resume", {"roundKey": "r2", "inputs": {}, "sessionReference": "fixture-session"})
        await self.until("interaction.request", "resume")

    async def test_invalid_suspend_reference_does_not_claim_suspension(self):
        await self.start(PROGRAM.replace('return "fixture-session"', 'return ""'))
        await self.ask()
        await self.send("execution.suspend", "suspend", {"roundKey": "r"})
        self.assertEqual((await self.until("error"))["payload"]["code"], "ADAPTER_SUSPEND_FAILED")
        await self.send("interaction.respond", "after-failed-suspend", self.answer())
        await self.until("interaction.received")
        self.assertEqual((await self.until("execution.finished"))["payload"]["outcome"], "FINISHED")
        self.assertFalse(any(f["method"] == "execution.suspended" for f in self.frames))

    async def test_disabled_and_unknown_capabilities_keep_control_available(self):
        program = '''
import asyncio
from seven_agent.force.agent import *
agent = AgentAdapter(name="minimal", version="1")
@agent.run
async def run(request, ctx):
    await ctx.started()
    await asyncio.Event().wait()
agent.serve()
'''
        ready = await self.start(program)
        self.assertFalse({"interaction.request", "user.message", "execution.suspend"} & set(ready["payload"]["capabilities"]))
        await self.send("execution.start", "start", {"roundKey": "r", "inputs": {}})
        await self.until("execution.started")
        for method, payload in (("interaction.respond", self.answer()),
                                ("user.message", {"roundKey": "r", "messageKey": "m", "text": "hi"}),
                                ("execution.suspend", {"roundKey": "r"}),
                                ("unknown.control", {})):
            await self.send(method, method, payload)
            self.assertEqual((await self.until("error"))["payload"]["code"], "ADAPTER_METHOD_UNSUPPORTED")
        await self.send("execution.interrupt", "stop", {"roundKey": "r"})
        self.assertEqual((await self.until("execution.finished"))["payload"]["outcome"], "CANCELLED")

    async def test_optional_and_text_answers_over_stdio(self):
        program = PROGRAM.replace(
            'Question("q", "Choose", kind="SINGLE_CHOICE", options=(QuestionOption("a", "Alpha"),), allow_free_text=False)',
            'Question("q", "Required text", allow_free_text=False), Question("optional", "Optional text", required=False)')
        await self.start(program)
        await self.ask()
        await self.send("interaction.respond", "missing-required", self.answer(answers=[]))
        self.assertEqual((await self.until("error"))["payload"]["code"], "ADAPTER_ANSWER_INVALID")
        await self.send("interaction.respond", "text-answer", self.answer(answers=[
            {"questionKey": "q", "selectedOptionKeys": [], "text": "A text answer"},
            {"questionKey": "optional", "selectedOptionKeys": [], "text": ""}]))
        receipt = await self.until("interaction.received")
        self.assertEqual(receipt["payload"]["responseId"], "101")
        self.assertEqual((await self.until("execution.finished"))["payload"]["outcome"], "FINISHED")

    async def test_interrupt_while_suspend_handler_waits(self):
        program = PROGRAM.replace('return "fixture-session"', 'await asyncio.Event().wait()')
        await self.start(program)
        await self.ask()
        await self.send("execution.suspend", "suspend", {"roundKey": "r"})
        await self.until("message")
        await self.send("interaction.respond", "late-answer", self.answer())
        self.assertEqual((await self.until("error"))["payload"]["code"], "ADAPTER_ROUND_SUSPENDING")
        await self.send("user.message", "late-message", {"roundKey": "r", "messageKey": "late", "text": "late"})
        self.assertEqual((await self.until("error"))["payload"]["code"], "ADAPTER_ROUND_SUSPENDING")
        await self.send("execution.interrupt", "stop", {"roundKey": "r"})
        await self.until("control.ack", "stop")
        self.assertEqual((await self.until("execution.finished"))["payload"]["outcome"], "CANCELLED")
        self.assertFalse(any(f["method"] == "execution.suspended" for f in self.frames))

    async def test_buffered_interrupt_does_not_leave_delivery_pending(self):
        await self.start()
        await self.ask()
        payload = {"roundKey": "r", "messageKey": "m", "text": "block"}
        for method, key, body in (("user.message", "msg", payload),
                                  ("execution.interrupt", "stop", {"roundKey": "r"})):
            self.process.stdin.write(encode(dict(protocol=VERSION, method=method, key=key, payload=body)))
        await self.process.stdin.drain()
        await self.until("execution.finished")
        await self.send("user.message", "retry", payload)
        self.assertEqual((await self.until("message.received", "retry"))["payload"]["status"], "UNKNOWN")

    async def test_cancelled_message_receipt_precedes_natural_finish(self):
        await self.start()
        await self.ask()
        await self.send("user.message", "blocked", {"roundKey": "r", "messageKey": "m", "text": "block"})
        await self.until("message")
        await self.send("interaction.respond", "answer", self.answer())
        await self.until("execution.finished")
        receipt = next(f for f in self.frames if f["method"] == "message.received")
        self.assertEqual(receipt["payload"]["status"], "UNKNOWN")
        self.assertLess(self.frames.index(receipt), len(self.frames) - 1)

    async def test_cancelled_message_receipt_precedes_suspended(self):
        await self.start()
        await self.ask()
        await self.send("user.message", "blocked", {"roundKey": "r", "messageKey": "m", "text": "block"})
        await self.until("message")
        await self.send("execution.suspend", "pause", {"roundKey": "r"})
        await self.until("execution.suspended")
        receipt = next(f for f in self.frames if f["method"] == "message.received")
        self.assertEqual(receipt["payload"]["status"], "UNKNOWN")
        self.assertLess(self.frames.index(receipt), len(self.frames) - 1)
        self.assertFalse(any(f["method"] == "execution.finished" for f in self.frames))


class ValidationTests(unittest.IsolatedAsyncioTestCase):
    async def test_close_interaction_emits_once_and_releases_local_waiter(self):
        frames = []

        async def emit(method, payload):
            frames.append((method, payload))

        context = ExecutionContext("round", emit, capabilities={
            "interaction.request", "interaction.respond", "interaction.received", "interaction.closed"})
        await context.started(native_pid=1)
        request = QuestionRequest("native", (Question("q", "Question"),), is_blocking=False)
        waiting = asyncio.create_task(context.ask(request))
        await asyncio.sleep(0)
        await context.close_interaction("native", reason="TURN_COMPLETED")
        with self.assertRaises(asyncio.CancelledError):
            await waiting
        self.assertEqual(frames[-1], ("interaction.closed", {
            "roundKey": "round", "interactionKey": "native", "reason": "TURN_COMPLETED"}))

    def test_complete_question_from_wire_preserves_typed_semantics(self):
        request = QuestionRequest("i", (
            Question("optional", "Optional", "Description", "MULTIPLE_CHOICE", False, True,
                     (QuestionOption("a", "Alpha"), QuestionOption("b", "Beta"))),
            Question("text", "Text", "", "TEXT", True, False),
        ))
        self.assertEqual(QuestionRequest.from_wire(request.wire()), request)
        for field in ("key", "title", "description", "kind", "required", "allowFreeText", "options"):
            invalid = request.wire()
            del invalid["questions"][0][field]
            with self.assertRaises(ProtocolError):
                QuestionRequest.from_wire(invalid)
        invalid = request.wire()
        invalid["unexpected"] = True
        with self.assertRaises(ProtocolError):
            QuestionRequest.from_wire(invalid)

    async def test_eight_pending_admission_same_key_and_next_after_resolution(self):
        frames = []
        async def emit(method, payload):
            frames.append((method, payload))
        ctx = ExecutionContext("r", emit, capabilities=("interaction.request",))
        await ctx.started()
        question = QuestionRequest("first", (Question("q", "Text"),))
        first = asyncio.create_task(ctx.ask(question))
        duplicate = asyncio.create_task(ctx.ask(question))
        await asyncio.sleep(0)
        others = [asyncio.create_task(ctx.ask(QuestionRequest(f"pending-{index}", question.questions))) for index in range(2, 9)]
        await asyncio.sleep(0)
        with self.assertRaises(ProtocolError) as error:
            await ctx.ask(QuestionRequest("ninth", question.questions))
        self.assertEqual(error.exception.code, "ADAPTER_INTERACTION_BUSY")
        answer = (InteractionAnswer("q", text="received"),)
        ctx._questions["first"][1].set_result(answer)
        self.assertEqual(await first, answer)
        self.assertEqual(await duplicate, answer)
        second = asyncio.create_task(ctx.ask(QuestionRequest("ninth", question.questions)))
        await asyncio.sleep(0)
        self.assertIn("ninth", [p["interactionKey"] for m,p in frames if m == "interaction.request"])
        ctx._close()
        with self.assertRaises(asyncio.CancelledError):
            await second
        await asyncio.gather(*others, return_exceptions=True)

    async def test_restored_typed_validation_cancellation_and_capability_gate(self):
        frames = []
        async def emit(method, payload):
            frames.append((method, payload))
        question = QuestionRequest("ask", (Question("q", "Text"),))
        expected = InteractionResponse("ask", "101", (InteractionAnswer("q", text="saved"),))
        with self.assertRaises(ProtocolError) as error:
            await ExecutionContext("r", emit).receive_restored_answer(question, expected)
        self.assertEqual(error.exception.code, "ADAPTER_METHOD_UNSUPPORTED")
        ctx = ExecutionContext("r", emit, capabilities=("interaction.request",))
        await ctx.started()
        for invalid in (
            InteractionResponse("ask", 101, expected.answers),
            InteractionResponse("ask", "0", expected.answers),
            InteractionResponse("ask", "9223372036854775808", expected.answers),
            InteractionResponse("ask", "101", (InteractionAnswer("q", text="x" * 16384),)),
            InteractionResponse("ask", "101", ()),
            InteractionResponse("other", "101", expected.answers),
        ):
            with self.assertRaises(ProtocolError):
                await ctx.receive_restored_answer(question, invalid)
            self.assertEqual(ctx._questions, {})
        pending = asyncio.create_task(ctx.receive_restored_answer(question, expected))
        await asyncio.sleep(0)
        self.assertIsNone(ctx.received_response("ask"))
        second = asyncio.create_task(ctx.ask(QuestionRequest("second", question.questions)))
        await asyncio.sleep(0)
        self.assertEqual([m for m,_ in frames], ["execution.started", "interaction.request"])
        pending.cancel()
        with self.assertRaises(asyncio.CancelledError):
            await pending
        self.assertTrue(ctx._questions["ask"][1].cancelled())
        second.cancel()
        await asyncio.gather(second, return_exceptions=True)

    def validate(self, method, payload, direction="up"):
        return decode(encode(dict(protocol=VERSION, method=method, key="test", payload=payload)), direction=direction)

    async def test_capabilities_and_direct_ask_gate(self):
        for caps in (("unknown",), ("user.message",), ("execution.suspend",), "interaction.request"):
            with self.assertRaises(ValueError):
                AgentAdapter(name="test", version="1", capabilities=caps)
        async def emit(*args):
            self.fail("disabled ask emitted")
        with self.assertRaises(ProtocolError):
            await ExecutionContext("r", emit).ask(QuestionRequest("i", (Question("q", "Title"),)))
        adapter = AgentAdapter(name="test", version="1")
        @adapter.suspend
        async def suspend(request, ctx):
            return "session"
        with self.assertRaises(ValueError):
            _ = adapter.capabilities

    async def test_ask_timeout_consumes_key_and_bounds_history(self):
        frames = []
        async def emit(method, payload):
            frames.append((method, payload))
        ctx = ExecutionContext("r", emit, capabilities=("interaction.request",))
        await ctx.started()
        request = QuestionRequest("i", (Question("q", "Title"),))
        with self.assertRaises(asyncio.TimeoutError):
            await asyncio.wait_for(ctx.ask(request), .01)
        with self.assertRaises(asyncio.CancelledError):
            await ctx.ask(request)
        with self.assertRaises(ProtocolError):
            await ctx.ask(QuestionRequest("i", (Question("q", "Changed"),)))
        pending = [asyncio.create_task(ctx.ask(QuestionRequest(f"other-{index}", (Question("q", "Title"),)))) for index in range(8)]
        await asyncio.sleep(0)
        with self.assertRaises(ProtocolError) as error:
            await ctx.ask(QuestionRequest("ninth", (Question("q", "Title"),)))
        self.assertEqual(error.exception.code, "ADAPTER_INTERACTION_BUSY")
        self.assertEqual(sum(method == "interaction.request" for method, _ in frames), 9)
        for task in pending:
            task.cancel()
        await asyncio.gather(*pending, return_exceptions=True)
        ctx._questions.update({"old:" + str(i): ctx._questions["i"] for i in range(255)})
        with self.assertRaises(ProtocolError) as error:
            await ctx.ask(QuestionRequest("new", (Question("q", "Title"),)))
        self.assertEqual(error.exception.code, "ADAPTER_SESSION_LIMIT")

    async def test_exact_question_option_and_answer_bounds(self):
        request = QuestionRequest("i", tuple(Question("q" + str(i), "Title", kind="MULTIPLE_CHOICE", options=tuple(
            QuestionOption("o" + str(j), "Label") for j in range(8))) for i in range(3))).wire()
        self.validate("interaction.request", {"roundKey": "r", **request})
        bad = copy.deepcopy(request)
        bad["questions"].append(copy.deepcopy(bad["questions"][0]))
        with self.assertRaises(ProtocolError):
            self.validate("interaction.request", {"roundKey": "r", **bad})
        bad = copy.deepcopy(request)
        bad["questions"][0]["options"].append({"key": "ninth", "label": "bad"})
        with self.assertRaises(ProtocolError):
            self.validate("interaction.request", {"roundKey": "r", **bad})
        answers = [{"questionKey": "q", "selectedOptionKeys": [], "text": ""}]
        overhead = len(encode(answers)) - 1
        answers[0]["text"] = "x" * (16384 - overhead)
        payload = {"roundKey": "r", "interactionKey": "i", "responseId": "101", "answers": answers}
        self.validate("interaction.respond", payload, "down")
        answers[0]["text"] += "中"
        with self.assertRaises(ProtocolError):
            self.validate("interaction.respond", payload, "down")

    async def test_strict_fields_and_direction(self):
        payloads = {
            "interaction.request": {"roundKey": "r", **QuestionRequest("i", (Question("q", "Title"),)).wire()},
            "interaction.respond": {"roundKey": "r", "interactionKey": "i", "responseId": "101", "answers": [{"questionKey": "q", "selectedOptionKeys": [], "text": "yes"}]},
            "interaction.received": {"roundKey": "r", "interactionKey": "i", "responseId": "101", "status": "RECEIVED"},
            "user.message": {"roundKey": "r", "messageKey": "m", "text": "hi"},
            "message.received": {"roundKey": "r", "messageKey": "m", "status": "UNKNOWN"},
            "execution.suspend": {"roundKey": "r"},
            "execution.suspended": {"roundKey": "r", "sessionReference": "s"},
        }
        for method, payload in payloads.items():
            direction = "down" if method in {"interaction.respond", "user.message", "execution.suspend"} else "up"
            self.validate(method, payload, direction)
            with self.assertRaises(ProtocolError):
                self.validate(method, payload, "up" if direction == "down" else "down")
            with self.assertRaises(ProtocolError):
                self.validate(method, {**payload, "recoverable": True}, direction)

    async def test_frozen_per_question_semantics(self):
        options = (QuestionOption("a", "Alpha"), QuestionOption("b", "Beta"))
        request = QuestionRequest("i", (
            Question("single", "Single", kind="SINGLE_CHOICE", options=options, allow_free_text=False),
            Question("multi", "Multi", kind="MULTIPLE_CHOICE", options=options, required=False),
            Question("text", "Text", kind="TEXT", allow_free_text=False, required=False),
        )).wire()
        def answer(name, selected=(), text=""):
            return {"questionKey": name, "selectedOptionKeys": list(selected), "text": text}
        for answers in (
            [answer("single", ("a",))],
            [answer("single", ("b",)), answer("multi")],
            [answer("single", ("a",)), answer("multi", ("a", "b")), answer("text", text="TEXT ignores allowFreeText")],
        ):
            validate_answers(request, answers_from_wire(answers))
        for answers in (
            [], [answer("single")], [answer("single", ("a", "b"))],
            [answer("single", text="free text forbidden")],
            [answer("single", ("a",)), answer("unknown")],
            [answer("single", ("a",)), answer("text", ("a",))],
            [answer("single", ("a",)), answer("multi", ("unknown",))],
            [answer("single", ("a",)), answer("multi", ("a", "a"))],
            [answer("single", ("a",)), answer("single", ("b",))],
        ):
            with self.subTest(answers=answers), self.assertRaises(ProtocolError):
                validate_answers(request, answers_from_wire(answers))
        optional = QuestionRequest("optional", (Question("q", "Optional", required=False),)).wire()
        validate_answers(optional, answers_from_wire([]))
        validate_answers(optional, answers_from_wire([answer("q")]))
        required_text = QuestionRequest("required", (Question("q", "Required"),)).wire()
        with self.assertRaises(ProtocolError):
            validate_answers(required_text, answers_from_wire([answer("q", text=" \t")]))

    async def test_question_text_bytes_identity_and_response_id(self):
        self.validate("interaction.request", {"roundKey": "r", **QuestionRequest("i", (
            Question("中文 key", "x" * 512, description="x" * 4096),)).wire()})
        for question in (Question("q", "x" * 513), Question("q", "Title", description="中" * 1366),
                         Question("q", "Title", kind="UNKNOWN"), Question("q", "Title", required=1),
                         Question("q", "Title", options=(QuestionOption("o", "Option"),)),
                         Question("q", "Title", kind="SINGLE_CHOICE"),
                         Question("q", "Title", kind="SINGLE_CHOICE", options=(QuestionOption("o", "x" * 513),))):
            with self.assertRaises(ProtocolError):
                question.wire()
        for response_id in (0, "0", "01", "-1", "9223372036854775808", "response-id"):
            with self.assertRaises(ProtocolError):
                self.validate("interaction.respond", {"roundKey": "r", "interactionKey": "i", "responseId": response_id, "answers": []}, "down")
        # Backend json.Marshal escapes HTML; count the same bytes at the limit.
        with self.assertRaises(ProtocolError):
            answers_from_wire([{"questionKey": "q", "selectedOptionKeys": [], "text": "<" * 2800}])
