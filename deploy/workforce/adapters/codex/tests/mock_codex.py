#!/usr/bin/env python3
"""Deterministic Codex App Server 0.147.0 protocol substitute; no provider."""
import json
import os
from pathlib import Path
import sys

SCHEMA_FIXTURE = json.loads((Path(__file__).parent / "schema_fixture_0_147_0.json").read_text())
Path("invocation.json").write_text(json.dumps({"args": sys.argv[1:], "env": dict(os.environ), "pid": os.getpid()}))
thread_id = "native-session-123"
turn_id = "turn-1"
pending_question = False
pending_tool = False
pending_approval = False
review = False


def send(value):
    print(json.dumps(value, separators=(",", ":")), flush=True)


def complete(status="completed", text="Public answer"):
    send({"method": "item/completed", "params": {"threadId": thread_id, "turnId": turn_id,
          "completedAtMs": 1, "item": {"id": "message-1", "type": "agentMessage", "text": text}}})
    send({"method": "turn/completed", "params": {"threadId": thread_id,
          "turn": {"id": turn_id, "items": [], "status": status}}})


for line in sys.stdin:
    frame = json.loads(line)
    method, params = frame.get("method"), frame.get("params", {})
    if method == "initialize":
        Path("initialize.json").write_text(json.dumps(params))
        send({"id": frame["id"], "result": {"userAgent": "mock-app-server/0.147.0"}})
    elif method == "initialized":
        continue
    elif method in ("thread/start", "thread/resume"):
        if method == "thread/resume":
            thread_id = params["threadId"]
        Path("thread.json").write_text(json.dumps(params))
        send({"id": frame["id"], "result": {"thread": {"id": thread_id, "turns": []},
              "model": params.get("model", "fixture"), "modelProvider": params.get("modelProvider", "seven"),
              "cwd": params.get("cwd", "/workspace"), "approvalPolicy": "never",
              "approvalsReviewer": "user", "sandbox": params.get("sandbox", "danger-full-access")}})
    elif method == "turn/start":
        Path("turn-start.json").write_text(json.dumps(params))
        if any(key in params for key in SCHEMA_FIXTURE["turnStartForbiddenFields"]):
            Path("rejected-turn-start.json").write_text(json.dumps(params))
            send({"id": frame["id"], "error": {"code": -32602, "message": "unsupported turn/start field"}})
            continue
        text = params["input"][0]["text"]
        review = "outputSchema" in params
        send({"id": frame["id"], "result": {"turn": {"id": turn_id, "items": [], "status": "inProgress"}}})
        send({"method": "turn/started", "params": {"threadId": thread_id, "turn": {"id": turn_id, "items": [], "status": "inProgress"}}})
        send({"method": "item/started", "params": {"threadId": thread_id, "turnId": turn_id,
              "startedAtMs": 1, "item": {"id": "command-1", "type": "commandExecution", "status": "inProgress", "command": "private", "commandActions": [], "cwd": "/workspace"}}})
        send({"method": "item/completed", "params": {"threadId": thread_id, "turnId": turn_id,
              "completedAtMs": 2, "item": {"id": "command-1", "type": "commandExecution", "status": "completed", "command": "private", "commandActions": [], "cwd": "/workspace", "exitCode": 0}}})
        if "REQUEST_NATIVE_APPROVAL" in text:
            pending_approval = True
            send({"id": 702, "method": "item/commandExecution/requestApproval", "params": {
                  "threadId": thread_id, "turnId": turn_id, "itemId": "command-approval-1",
                  "command": "curl https://registry.npmjs.org", "cwd": "/workspace"}})
        elif "CALL_NETWORK_TOOL" in text:
            pending_tool = True
            arguments = {"purpose": "Install locked dependencies", "command": "pnpm install --frozen-lockfile",
                         "cwd": ".", "targets": [{"host": "registry.npmjs.org", "protocol": "https", "port": 443}]}
            send({"method": "item/started", "params": {"threadId": thread_id, "turnId": turn_id,
                  "item": {"id": "tool-call-1", "type": "dynamicToolCall", "status": "inProgress",
                           "tool": "run_command_with_network_access", "arguments": arguments}}})
            send({"id": 701, "method": "item/tool/call", "params": {"threadId": thread_id,
                  "turnId": turn_id, "callId": "tool-call-1", "tool": "run_command_with_network_access",
                  "namespace": None, "arguments": arguments}})
        elif "CALL_GIT_DELIVERY_TOOL" in text:
            pending_tool = True
            arguments = {"summary": "Pagination boundary fix and regression checks are complete."}
            send({"method": "item/started", "params": {"threadId": thread_id, "turnId": turn_id,
                  "item": {"id": "git-delivery-call-1", "type": "dynamicToolCall",
                           "status": "inProgress", "tool": "request_git_delivery",
                           "arguments": arguments}}})
            send({"id": 703, "method": "item/tool/call", "params": {"threadId": thread_id,
                  "turnId": turn_id, "callId": "git-delivery-call-1",
                  "tool": "request_git_delivery", "namespace": None, "arguments": arguments}})
        elif "ASK_NATIVE_QUESTION" in text or "ASK_NATIVE_DEFAULT" in text or "ASK_NATIVE_NONBLOCKING" in text or "ASK_NATIVE_SECRET" in text or "NATIVE_REQUEST_RESOLVED" in text:
            if "features.default_mode_request_user_input=true" not in sys.argv:
                complete(text="request_user_input is unavailable in Default mode")
                continue
            pending_question = True
            send({"id": 700, "method": "item/tool/requestUserInput", "params": {"threadId": thread_id,
                  "turnId": turn_id, "itemId": "question-1", "isBlocking": not any(marker in text for marker in ("ASK_NATIVE_DEFAULT", "ASK_NATIVE_NONBLOCKING")),
                  "questions": [{"id": "scope", "header": "Scope", "question": "Choose scope",
                                  "isOther": True, "isSecret": "ASK_NATIVE_SECRET" in text,
                                  "options": [{"label": "Small", "description": "Only target files"}]}]}})
            if "NATIVE_REQUEST_RESOLVED" in text:
                send({"method": "serverRequest/resolved", "params": {"threadId": thread_id, "requestId": 700}})
                complete(text="Native request cleared")
            elif "ASK_NATIVE_NONBLOCKING" in text:
                complete(text="Continued without waiting")
        else:
            body = '{"schemaVersion":"review-report/v1","verdict":"PASS","summary":"fixture"}' if review else "Public answer"
            complete(text=body)
    elif method == SCHEMA_FIXTURE["injectMethod"]:
        Path("inject-items.json").write_text(json.dumps(params))
        valid = set(params) == {"threadId", "items"} and params.get("threadId") == thread_id
        items = params.get("items")
        valid = valid and isinstance(items, list) and len(items) == 1
        item = items[0] if valid else {}
        valid = valid and isinstance(item, dict) and set(item) == {
            "type", "call_id", "name", "output"}
        valid = valid and item.get("type") == SCHEMA_FIXTURE["outputItemType"]
        valid = valid and item.get("name") == SCHEMA_FIXTURE["toolName"]
        valid = valid and isinstance(item.get("call_id"), str) and bool(item["call_id"])
        output = item.get("output")
        valid = valid and isinstance(output, list) and len(output) == 1
        content = output[0] if valid else {}
        valid = valid and isinstance(content, dict) and set(content) == {"type", "text"}
        valid = valid and content.get("type") == SCHEMA_FIXTURE["outputContentType"]
        try:
            json.loads(content.get("text", ""))
        except (TypeError, ValueError):
            valid = False
        if not valid:
            send({"id": frame["id"], "error": {"code": -32602, "message": "invalid injected item"}})
            continue
        if item["call_id"] == "native-call-response-loss":
            # The item was observed, but the client cannot know whether it was
            # committed. Simulate a connection loss before the response.
            raise SystemExit(0)
        send({"id": frame["id"], "result": {}})
    elif method == "turn/steer":
        send({"id": frame["id"], "result": {"turnId": turn_id}})
    elif method == "turn/interrupt":
        send({"id": frame["id"], "result": {}})
        complete(status="interrupted", text="Interrupted")
    elif "id" in frame and pending_approval:
        pending_approval = False
        Path("approval-result.json").write_text(json.dumps(frame))
        complete(text="Native approval remained unavailable")
    elif "id" in frame and pending_tool:
        pending_tool = False
        Path("tool-result.json").write_text(json.dumps(frame))
        result = frame.get("result", {})
        contents = result.get("contentItems")
        if (not isinstance(contents, list) or len(contents) != 1
                or contents[0].get("type") != SCHEMA_FIXTURE["dynamicToolOutputContentType"]):
            Path("invalid-tool-result-shape.json").write_text(json.dumps(frame))
            complete(status="failed", text="Invalid dynamic tool response shape")
            continue
        request_id = 703 if "CALL_GIT_DELIVERY_TOOL" in Path("turn-start.json").read_text() else 701
        tool_name = "request_git_delivery" if request_id == 703 else "run_command_with_network_access"
        call_id = "git-delivery-call-1" if request_id == 703 else "tool-call-1"
        send({"method": "serverRequest/resolved", "params": {"threadId": thread_id, "requestId": request_id}})
        send({"method": "item/completed", "params": {"threadId": thread_id, "turnId": turn_id,
              "item": {"id": call_id, "type": "dynamicToolCall", "status": "completed",
                       "tool": tool_name, "arguments": {},
                       "success": result.get("success"), "contentItems": result.get("contentItems")}}})
        complete(text="Continued after dynamic tool")
    elif "id" in frame and pending_question:
        pending_question = False
        Path("question-result.json").write_text(json.dumps(frame))
        complete(text="Continued after answer")
    elif "id" in frame:
        send({"id": frame["id"], "error": {"code": -32601, "message": "unsupported"}})
