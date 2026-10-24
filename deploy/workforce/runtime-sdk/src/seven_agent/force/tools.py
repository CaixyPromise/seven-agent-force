"""Typed, bounded Adapter tool contracts. Tool calls never grant authority."""
from __future__ import annotations

import dataclasses
import enum
import hashlib
import ipaddress
import json
import posixpath
import re

from ._protocol import ProtocolError, encode

MAX_ARGUMENTS_BYTES = 64 << 10
MAX_OUTPUT_BYTES = 64 << 10
MAX_TARGETS = 8
MAX_GIT_DELIVERY_SUMMARY_BYTES = 2 << 10
NETWORK_COMMAND_TOOL_NAME = "run_command_with_network_access"
GIT_DELIVERY_TOOL_NAME = "request_git_delivery"
TOOL_METHODS = frozenset({"tool.catalog", "tool.request", "tool.result", "tool.received", "tool.closed"})
TOOL_NAME = re.compile(r"[a-z][a-z0-9_]{0,63}")
WIRE_KEY = re.compile(r"[A-Za-z0-9._:-]{1,128}")
HEX_DIGEST = re.compile(r"[0-9a-f]{64}")
HOSTNAME = re.compile(r"(?=.{1,253}\Z)[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?(?:\.[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?)+")
REASON_CODE = re.compile(r"[A-Z][A-Z0-9_]{0,127}")


def _fields(value, expected):
    if not isinstance(value, dict) or set(value) != set(expected.split()):
        raise ProtocolError()


def _key(value):
    if not isinstance(value, str) or WIRE_KEY.fullmatch(value) is None:
        raise ProtocolError()


def _native_call_id(value):
    if not isinstance(value, str) or not value:
        raise ProtocolError("ADAPTER_TOOL_NATIVE_ID_INVALID")
    try:
        if len(value.encode("utf-8")) > 512:
            raise ProtocolError("ADAPTER_TOOL_NATIVE_ID_INVALID")
    except UnicodeError:
        raise ProtocolError("ADAPTER_TOOL_NATIVE_ID_INVALID") from None
    if any(ord(character) < 32 or ord(character) == 127 for character in value):
        raise ProtocolError("ADAPTER_TOOL_NATIVE_ID_INVALID")


def _text(value, limit, *, nonempty=False):
    if not isinstance(value, str) or "\0" in value or (nonempty and not value.strip()):
        raise ProtocolError()
    try:
        if len(value.encode("utf-8")) > limit:
            raise ProtocolError()
    except UnicodeError:
        raise ProtocolError() from None


def _go_json(value):
    """Match encoding/json for the normalized string-only tool structure."""
    try:
        raw = json.dumps(value, ensure_ascii=False, allow_nan=False,
                         separators=(",", ":"))
    except (ValueError, TypeError, UnicodeError, RecursionError):
        raise ProtocolError() from None
    raw = raw.replace("&", "\\u0026").replace("<", "\\u003c").replace(">", "\\u003e")
    raw = raw.replace("\u2028", "\\u2028").replace("\u2029", "\\u2029")
    return raw.encode("utf-8")


def _network_digest(arguments):
    ordered = {
        "purpose": arguments["purpose"],
        "command": arguments["command"],
        "cwd": arguments["cwd"],
        "targets": [
            {"host": target["host"], "protocol": target["protocol"], "port": target["port"]}
            for target in arguments["targets"]
        ],
    }
    return hashlib.sha256(_go_json(ordered)).hexdigest()


def _network_arguments(value):
    _fields(value, "purpose command cwd targets")
    purpose = value["purpose"].strip() if isinstance(value["purpose"], str) else value["purpose"]
    command = value["command"].strip() if isinstance(value["command"], str) else value["command"]
    cwd = value["cwd"].strip() if isinstance(value["cwd"], str) else value["cwd"]
    cwd = cwd or "."
    _text(purpose, 2048, nonempty=True)
    _text(command, 16 << 10, nonempty=True)
    _text(cwd, 4096, nonempty=True)
    if (cwd.startswith("/") or posixpath.normpath(cwd) != cwd
            or cwd == ".." or cwd.startswith("../")
            or any(ord(character) < 32 for character in cwd)):
        raise ProtocolError("ADAPTER_TOOL_REQUEST_INVALID")
    if not isinstance(value["targets"], list) or not 1 <= len(value["targets"]) <= MAX_TARGETS:
        raise ProtocolError("ADAPTER_TOOL_REQUEST_INVALID")
    targets = [ToolTarget.from_wire(target).wire() for target in value["targets"]]
    if len({(target["host"], target["protocol"], target["port"]) for target in targets}) != len(targets):
        raise ProtocolError("ADAPTER_TOOL_REQUEST_INVALID")
    if len({target["host"] for target in targets}) != len(targets):
        raise ProtocolError("ADAPTER_TOOL_REQUEST_INVALID")
    targets.sort(key=lambda target: (target["host"], target["protocol"], target["port"]))
    return {"purpose": purpose, "command": command, "cwd": cwd, "targets": targets}


def _git_delivery_arguments(value):
    _fields(value, "summary")
    summary = value["summary"].strip() if isinstance(value["summary"], str) else value["summary"]
    _text(summary, MAX_GIT_DELIVERY_SUMMARY_BYTES, nonempty=True)
    return {"summary": summary}


def _tool_arguments(tool_name, value):
    if tool_name == NETWORK_COMMAND_TOOL_NAME:
        normalized = _network_arguments(value)
    elif tool_name == GIT_DELIVERY_TOOL_NAME:
        normalized = _git_delivery_arguments(value)
    else:
        raise ProtocolError("ADAPTER_TOOL_REQUEST_INVALID")
    return normalized, hashlib.sha256(_go_json(normalized)).hexdigest()


class ToolDeliveryStatus(str, enum.Enum):
    RECEIVED = "RECEIVED"
    REJECTED = "REJECTED"
    UNKNOWN = "UNKNOWN"


@dataclasses.dataclass(frozen=True)
class ToolTarget:
    host: str
    protocol: str
    port: int

    @classmethod
    def from_wire(cls, value):
        _fields(value, "host protocol port")
        host = value["host"].strip().removesuffix(".").lower() if isinstance(value["host"], str) else value["host"]
        protocol = value["protocol"].strip().lower() if isinstance(value["protocol"], str) else value["protocol"]
        item = cls(host, protocol, value["port"])
        item.wire()
        return item

    def wire(self):
        _text(self.host, 253, nonempty=True)
        if HOSTNAME.fullmatch(self.host) is None:
            raise ProtocolError("ADAPTER_TOOL_TARGET_INVALID")
        try:
            ipaddress.ip_address(self.host.rstrip("."))
        except ValueError:
            pass
        else:
            raise ProtocolError("ADAPTER_TOOL_TARGET_INVALID")
        if self.protocol not in ("http", "https") or type(self.port) is not int or not 1 <= self.port <= 65535:
            raise ProtocolError("ADAPTER_TOOL_TARGET_INVALID")
        return {"host": self.host, "protocol": self.protocol, "port": self.port}


@dataclasses.dataclass(frozen=True)
class ToolDefinition:
    name: str
    description: str
    input_schema: dict

    def wire(self):
        if not isinstance(self.name, str) or TOOL_NAME.fullmatch(self.name) is None:
            raise ProtocolError("ADAPTER_TOOL_DEFINITION_INVALID")
        _text(self.description, 4096, nonempty=True)
        if (not isinstance(self.input_schema, dict)
                or self.input_schema.get("type") != "object"
                or self.input_schema.get("additionalProperties") is not False
                or len(encode(self.input_schema)) > 16 << 10):
            raise ProtocolError("ADAPTER_TOOL_DEFINITION_INVALID")
        return {"type": "function", "name": self.name, "description": self.description,
                "inputSchema": self.input_schema}


@dataclasses.dataclass(frozen=True)
class ToolCallRequest:
    native_tool_call_id: str
    local_tool_call_key: str
    tool_name: str
    arguments: dict

    @classmethod
    def from_wire(cls, value):
        _fields(value, "nativeToolCallId localToolCallKey toolName arguments argumentsDigest")
        item = cls(value["nativeToolCallId"], value["localToolCallKey"],
                   value["toolName"], value["arguments"])
        wire = item.wire()
        if wire["argumentsDigest"] != value["argumentsDigest"]:
            raise ProtocolError("ADAPTER_TOOL_DIGEST_MISMATCH")
        return item

    def wire(self):
        _native_call_id(self.native_tool_call_id)
        _key(self.local_tool_call_key)
        if not isinstance(self.arguments, dict) or len(encode(self.arguments)) > MAX_ARGUMENTS_BYTES:
            raise ProtocolError("ADAPTER_TOOL_REQUEST_INVALID")
        arguments, digest = _tool_arguments(self.tool_name, self.arguments)
        return {"nativeToolCallId": self.native_tool_call_id,
                "localToolCallKey": self.local_tool_call_key, "toolName": self.tool_name,
                "arguments": arguments, "argumentsDigest": digest}


@dataclasses.dataclass(frozen=True)
class ToolResult:
    native_tool_call_id: str
    local_tool_call_key: str
    operation_id: str
    status: str
    exit_code: int | None = None
    summary: str = ""
    output: str = ""
    result_digest: str = ""
    reason_code: str = ""

    @classmethod
    def from_wire(cls, value):
        _fields(value, "nativeToolCallId localToolCallKey operationId status exitCode summary output resultDigest reasonCode")
        item = cls(value["nativeToolCallId"], value["localToolCallKey"],
                   value["operationId"], value["status"],
                   value["exitCode"], value["summary"], value["output"],
                   value["resultDigest"], value["reasonCode"])
        item.wire()
        return item

    def wire(self):
        _native_call_id(self.native_tool_call_id)
        _key(self.local_tool_call_key)
        if (not isinstance(self.operation_id, str)
                or re.fullmatch(r"[1-9][0-9]{0,18}", self.operation_id) is None
                or int(self.operation_id) > (1 << 63) - 1):
            raise ProtocolError("ADAPTER_TOOL_RESULT_INVALID")
        if self.status not in ("SUCCEEDED", "FAILED", "SCHEDULED", "DENIED", "OUTCOME_UNKNOWN", "CANCELLED", "EXPIRED"):
            raise ProtocolError("ADAPTER_TOOL_RESULT_INVALID")
        if self.exit_code is not None and (type(self.exit_code) is not int or not -(1 << 31) <= self.exit_code < (1 << 31)):
            raise ProtocolError("ADAPTER_TOOL_RESULT_INVALID")
        _text(self.summary, 2048)
        _text(self.output, MAX_OUTPUT_BYTES)
        if self.result_digest and HEX_DIGEST.fullmatch(self.result_digest) is None:
            raise ProtocolError("ADAPTER_TOOL_RESULT_INVALID")
        if self.reason_code and REASON_CODE.fullmatch(self.reason_code) is None:
            raise ProtocolError("ADAPTER_TOOL_RESULT_INVALID")
        if self.status in {"SUCCEEDED", "FAILED"} and (
                not self.result_digest or self.exit_code is None or self.reason_code):
            raise ProtocolError("ADAPTER_TOOL_RESULT_INVALID")
        if self.status == "SCHEDULED" and (
                self.exit_code is not None or not self.summary.strip() or self.output
                or not self.result_digest or self.reason_code):
            raise ProtocolError("ADAPTER_TOOL_RESULT_INVALID")
        if self.status in {"DENIED", "OUTCOME_UNKNOWN", "CANCELLED", "EXPIRED"} and (
                self.exit_code is not None or self.output or self.result_digest or not self.reason_code):
            raise ProtocolError("ADAPTER_TOOL_RESULT_INVALID")
        return {"nativeToolCallId": self.native_tool_call_id,
                "localToolCallKey": self.local_tool_call_key, "operationId": self.operation_id,
                "status": self.status, "exitCode": self.exit_code, "summary": self.summary,
                "output": self.output, "resultDigest": self.result_digest,
                "reasonCode": self.reason_code}


def validate_payload(method, payload):
    if method not in TOOL_METHODS:
        return
    expected = {
        "tool.catalog": "roundKey toolNames catalogDigest",
        "tool.request": "roundKey nativeToolCallId localToolCallKey toolName arguments argumentsDigest",
        "tool.result": "roundKey nativeToolCallId localToolCallKey operationId status exitCode summary output resultDigest reasonCode",
        "tool.received": "roundKey localToolCallKey operationId status",
        "tool.closed": "roundKey localToolCallKey reason",
    }
    _fields(payload, expected[method])
    _key(payload["roundKey"])
    if method == "tool.catalog":
        names = payload["toolNames"]
        if (not isinstance(names, list) or len(names) > 16
                or names != sorted(names) or len(set(names)) != len(names)
                or any(not isinstance(name, str) or TOOL_NAME.fullmatch(name) is None for name in names)):
            raise ProtocolError("ADAPTER_TOOL_CATALOG_INVALID")
        body = json.dumps(names, ensure_ascii=True, separators=(",", ":")).encode("utf-8")
        if payload["catalogDigest"] != hashlib.sha256(body).hexdigest():
            raise ProtocolError("ADAPTER_TOOL_CATALOG_INVALID")
        return
    _key(payload["localToolCallKey"])
    if method == "tool.request":
        ToolCallRequest.from_wire({key: payload[key] for key in expected[method].split() if key != "roundKey"})
    elif method == "tool.result":
        ToolResult.from_wire({key: payload[key] for key in expected[method].split() if key != "roundKey"})
    elif method == "tool.received":
        if (not isinstance(payload["operationId"], str)
                or re.fullmatch(r"[1-9][0-9]{0,18}", payload["operationId"]) is None
                or int(payload["operationId"]) > (1 << 63) - 1
                or payload["status"] not in {item.value for item in ToolDeliveryStatus}):
            raise ProtocolError("ADAPTER_TOOL_RECEIPT_INVALID")
    elif not isinstance(payload["reason"], str) or REASON_CODE.fullmatch(payload["reason"]) is None:
        raise ProtocolError("ADAPTER_TOOL_CLOSE_INVALID")
