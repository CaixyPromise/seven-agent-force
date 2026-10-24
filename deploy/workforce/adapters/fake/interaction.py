"""Fixed Batch3 safe-point fixture. No task-selected behavior or provider IO."""
from __future__ import annotations

import asyncio
import hashlib
import json
import os
import re
from pathlib import Path

from adapter import directory, read, write, regular, decode, require, validate, path, identifier
from seven_agent.force.agent import (AgentAdapter, ActivityStatus, DeliveryStatus, RoundResult,
                                           QuestionRequest, Question, QuestionOption, InteractionResponse)
from seven_agent.force.interaction import answers_from_wire, validate_answers, text, key

SESSION_FILE = "fake-session.json"
MAX_MESSAGES = 32
MAX_MESSAGE_BYTES = 64 << 10


def load_fixture(asset_digest):
    asset = Path(__file__).absolute().parent / "fixtures" / "batch3.json"
    with directory(asset.parent) as fd:
        raw = read(fd, asset.name)
    require(hashlib.sha256(raw).hexdigest() == asset_digest)
    fixture = decode(raw)
    require(set(fixture) == {"role", "scenario", "interactionKey", "questions"})
    require(fixture["role"] == "AGENT" and fixture["scenario"] == "BATCH3_SAFE_POINT")
    return fixture


def body(value):
    return json.dumps(value, ensure_ascii=False, sort_keys=True, separators=(",", ":")).encode()


def response(value, question):
    require(isinstance(value, dict) and set(value) == {"interactionKey", "responseId", "answers"})
    require(value["interactionKey"] == question["interactionKey"])
    identifier(value["responseId"])
    validate_answers(question, answers_from_wire(value["answers"]))
    return decode(body(value))


class Execution:
    def __init__(self, request, context, fixture, digest, resume):
        self.context = context
        self.inputs = dict(request.inputs)
        has_cold = any(name in self.inputs for name in ("interactionResponse", "interactionQuestionRequest"))
        self.cold_response = self.inputs.pop("interactionResponse", None)
        self.cold_question = self.inputs.pop("interactionQuestionRequest", None)
        require(resume or not has_cold)
        require((self.cold_response is None) == (self.cold_question is None))
        require(not has_cold or self.cold_response is not None)
        validate(self.inputs, "AGENT")
        self.question = QuestionRequest(fixture["interactionKey"], tuple(
            Question(q["key"], q["title"], q["description"], q["kind"], q["required"],
                     q["allowFreeText"], tuple(QuestionOption(**o) for o in q["options"]))
            for q in fixture["questions"]))
        if self.cold_question is not None:
            trusted_question = QuestionRequest.from_wire(self.cold_question)
            require(trusted_question.wire() == self.question.wire())
            self.question = trusted_question
        self.question_wire = self.question.wire()
        self.state_root = path(self.inputs["state"]) / "codex"
        self.lock = asyncio.Lock()
        self.wake = asyncio.Event()
        self.pending = []
        self.message_completions = []
        self.paused = False
        self.ask_task = None
        self.closed = False
        with directory(self.state_root.parent) as fd:
            require(os.fstat(fd).st_mode & 0o077 == 0)
            try:
                os.mkdir("codex", 0o700, dir_fd=fd)
            except FileExistsError:
                pass
        with directory(self.state_root) as fd:
            require(os.fstat(fd).st_mode & 0o077 == 0)
            exists = regular(fd, SESSION_FILE, missing=True)
            if resume:
                require(exists is not None)
                session = decode(read(fd, SESSION_FILE, private=True))
                self.validate_session(session, digest, request.session_reference)
                session["round"] += 1
            else:
                require(exists is None)
                reference = "fake-session-" + hashlib.sha256(("batch3\0" + self.inputs["taskId"] + "\0" + self.inputs["attemptId"]).encode()).hexdigest()
                session = dict(schemaVersion=1, kind="fake-session", sessionId=reference,
                               sessionDigest=hashlib.sha256(reference.encode()).hexdigest(),
                               round=1, taskId=self.inputs["taskId"], fixtureDigest=digest,
                               question=self.question_wire, answer=None, supplements=[],
                               phase="ACTIVE", resultWritten=False)
            if self.cold_response is not None:
                saved = response(self.cold_response, self.question_wire)
                require(session["answer"] is None or session["answer"] == saved)
                # Private restore data is only an expected-value constraint.
                # Persist a previously absent answer at the normal post-receipt
                # safe point, never by accepting constructor input alone.
                self.cold_response = saved
            self.session = session
            if not session["resultWritten"]:
                session["phase"] = "ACTIVE"
            write(fd, SESSION_FILE, body(session))

    def validate_session(self, session, digest, reference):
        require(isinstance(session, dict) and set(session) == {
            "schemaVersion", "kind", "sessionId", "sessionDigest", "round", "taskId", "fixtureDigest",
            "question", "answer", "supplements", "phase", "resultWritten"})
        require(session["schemaVersion"] == 1 and session["kind"] == "fake-session")
        require(isinstance(reference, str) and re.fullmatch(r"fake-session-[0-9a-f]{64}", reference) is not None)
        require(session["sessionId"] == reference and session["sessionDigest"] == hashlib.sha256(reference.encode()).hexdigest())
        require(session["taskId"] == self.inputs["taskId"] and session["fixtureDigest"] == digest)
        require(type(session["round"]) is int and 1 <= session["round"] < 256)
        require(session["question"] == self.question_wire)
        require(session["phase"] in {"ACTIVE", "SUSPENDED", "FINISHED"} and type(session["resultWritten"]) is bool)
        require(session["resultWritten"] == (session["phase"] == "FINISHED"))
        if session["answer"] is not None:
            response(session["answer"], self.question_wire)
        require(isinstance(session["supplements"], list) and len(session["supplements"]) <= MAX_MESSAGES)
        seen = set()
        for item in session["supplements"]:
            require(isinstance(item, dict) and set(item) == {"messageKey", "text"})
            key(item["messageKey"])
            text(item["text"], 16 << 10, nonempty=True)
            require(item["messageKey"] not in seen)
            seen.add(item["messageKey"])
        require(len(body(session["supplements"])) <= MAX_MESSAGE_BYTES)
        require(not session["resultWritten"] or (session["answer"] is not None and session["supplements"]))

    def persist(self):
        with directory(self.state_root) as fd:
            write(fd, SESSION_FILE, body(self.session))

    def safe_point(self):
        """Caller holds lock. All writes are synchronous and stop before return."""
        received = self.context.received_response(self.question.interaction_key)
        if received is not None:
            saved = {"interactionKey": received.interaction_key, "responseId": received.response_id,
                     "answers": [answer.wire() for answer in received.answers]}
            require(self.session["answer"] is None or self.session["answer"] == saved)
            self.session["answer"] = saved
        pending, self.pending = self.pending, []
        try:
            for item, _ in pending:
                self.session["supplements"].append(item)
            self.persist()
        except BaseException:
            for _, future in pending:
                if not future.done():
                    future.set_result(DeliveryStatus.UNKNOWN)
            raise
        for _, future in pending:
            if not future.done():
                future.set_result(DeliveryStatus.RECEIVED)

    async def run(self):
        await self.context.started()
        await self.context.emit_activity(key="fake:prepare", title="Read and validate fixed fixture",
                                         status=ActivityStatus.STARTED)
        fixture = load_fixture(self.session["fixtureDigest"])
        require(QuestionRequest.from_wire({"interactionKey": fixture["interactionKey"],
                                          "questions": fixture["questions"],
                                          "isBlocking": True}).wire() == self.question_wire)
        await self.context.emit_activity(key="fake:prepare", title="Read and validate fixed fixture",
                                         status=ActivityStatus.COMPLETED)
        await self.context.emit_message("Fixed fixture validated; the original session is ready for the answer and supplement.")
        await self.context.emit_plan(["Receive the fixed question answer and supplement",
                                      "Record both result files and the original session at a safe point"])
        if self.cold_response is not None:
            expected = InteractionResponse(self.cold_response["interactionKey"], self.cold_response["responseId"],
                                           answers_from_wire(self.cold_response["answers"]))
            await self.context.receive_restored_answer(self.question, expected)
        if self.session["resultWritten"]:
            return RoundResult.finished(summary="Fixed result already recorded", session_reference=self.session["sessionId"])
        if self.session["answer"] is None:
            self.ask_task = asyncio.create_task(self.context.ask(self.question))
            self.ask_task.add_done_callback(lambda _: self.wake.set())
        try:
            while True:
                async with self.lock:
                    if self.paused:
                        await asyncio.Event().wait()
                    self.wake.clear()
                    self.safe_point()
                    if self.ask_task is not None and self.ask_task.done():
                        self.ask_task.result()
                    if self.session["answer"] is not None and self.session["supplements"]:
                        await self.context.emit_activity(key="fake:record", title="Record answer and supplement",
                                                         status=ActivityStatus.STARTED)
                        answer = self.session["answer"]["answers"][0]
                        selection = answer["selectedOptionKeys"][0]
                        with directory(path(self.inputs["workspace"])) as repo:
                            write(repo, "batch3-result.txt", ("Fixed answer: " + selection + "\n").encode())
                            write(repo, "batch3-supplements.txt", "\n".join(item["text"] for item in self.session["supplements"]).encode() + b"\n")
                        self.session["phase"] = "FINISHED"
                        self.session["resultWritten"] = True
                        self.closed = True
                        self.persist()
                        await self.context.emit_activity(key="fake:record", title="Record answer and supplement",
                                                         status=ActivityStatus.SUCCEEDED)
                        break
                await self.wake.wait()
            # A safe-point write is complete before finish can cancel handlers.
            await asyncio.gather(*(event.wait() for event in self.message_completions))
            await self.context.emit_message("Fixed answer and supplement recorded in the original session.")
            return RoundResult.finished(summary="Fixed answer and supplement recorded", session_reference=self.session["sessionId"])
        finally:
            self.closed = True
            if self.ask_task is not None:
                self.ask_task.cancel()
                await asyncio.gather(self.ask_task, return_exceptions=True)
            for _, future in self.pending:
                if not future.done():
                    future.set_result(DeliveryStatus.UNKNOWN)

    async def message(self, request):
        async with self.lock:
            if self.paused or self.closed:
                return DeliveryStatus.REJECTED
            item = {"messageKey": request.message_key, "text": request.text}
            for old in self.session["supplements"]:
                if old["messageKey"] == request.message_key:
                    return DeliveryStatus.RECEIVED if old == item else DeliveryStatus.REJECTED
            all_items = self.session["supplements"] + [p[0] for p in self.pending] + [item]
            if len(all_items) > MAX_MESSAGES or len(body(all_items)) > MAX_MESSAGE_BYTES:
                return DeliveryStatus.REJECTED
            future = asyncio.get_running_loop().create_future()
            completed = asyncio.Event()
            self.message_completions.append(completed)
            self.pending.append((item, future))
            self.wake.set()
        try:
            return await asyncio.shield(future)
        finally:
            completed.set()

    async def suspend(self):
        async with self.lock:
            require(not self.paused and not self.closed)
            self.safe_point()
            self.session["phase"] = "SUSPENDED"
            self.persist()
            self.paused = True
            if self.ask_task is not None:
                self.ask_task.cancel()
        await asyncio.gather(*(event.wait() for event in self.message_completions))
        return self.session["sessionId"]


def build_adapter(asset_digest, *, review_handler=None):
    fixture = load_fixture(asset_digest)
    agent = AgentAdapter(name="fake", version="batch3", capabilities=("interaction.request",))
    executions = {}

    async def execute(request, context, resume):
        execution = Execution(request, context, fixture, asset_digest, resume)
        executions[context] = execution
        try:
            return await execution.run()
        finally:
            executions.pop(context, None)

    @agent.run
    async def run(request, context):
        if review_handler is not None and request.inputs.get("mode") == "REVIEW":
            return await review_handler(request, context)
        return await execute(request, context, False)

    @agent.resume
    async def resume(request, context):
        return await execute(request, context, True)

    @agent.message
    async def message(request, context):
        if context not in executions:
            return DeliveryStatus.REJECTED
        return await executions[context].message(request)

    @agent.suspend
    async def suspend(request, context):
        return await executions[context].suspend()

    return agent
