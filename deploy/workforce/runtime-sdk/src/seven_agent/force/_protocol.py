"""Bounded Adapter wire codec; errors never echo untrusted input."""
import json
import math
import re

VERSION = "seven.agent-adapter/v1"
MAX_FRAME_BYTES = 1 << 20
MAX_TEXT_BYTES = 64 << 10
METHODS = {
    "down": frozenset(("initialize", "execution.start", "execution.resume", "execution.interrupt",
                       "interaction.respond", "user.message", "execution.suspend", "tool.result")),
    "up": frozenset(("ready", "execution.started", "execution.finished", "message", "plan", "activity", "control.ack", "error",
                     "interaction.request", "interaction.received", "interaction.closed", "message.received", "execution.suspended",
                     "tool.catalog", "tool.request", "tool.received", "tool.closed")),
}
ENTITY_IDS = frozenset(("taskId", "taskTurnId", "attemptId", "reviewRunId", "reviewRunAttemptId", "responseId", "operationId"))


class ProtocolError(ValueError):
    def __init__(self, code="ADAPTER_PROTOCOL_INVALID"):
        self.code = code
        super().__init__(code)


def _pairs(pairs):
    result = {}
    for key, value in pairs:
        if key in result:
            raise ProtocolError()
        result[key] = value
    return result


def _invalid_constant(_):
    raise ProtocolError()


def _payload(value, depth=0):
    if depth > 34:
        raise ProtocolError()
    if isinstance(value, float) and not math.isfinite(value):
        raise ProtocolError()
    if isinstance(value, dict):
        for key, item in value.items():
            if key in ENTITY_IDS:
                if not isinstance(item, str) or not re.fullmatch(r"[1-9][0-9]{0,18}", item) or int(item) > (1 << 63) - 1:
                    raise ProtocolError()
            _payload(item, depth + 1)
    elif isinstance(value, list):
        for item in value:
            _payload(item, depth + 1)


def encode(value):
    try:
        raw = (json.dumps(value, ensure_ascii=False, allow_nan=False,
                          sort_keys=True, separators=(",", ":")) + "\n").encode("utf-8")
    except (ValueError, TypeError, UnicodeError, RecursionError):
        raise ProtocolError() from None
    if len(raw) > MAX_FRAME_BYTES:
        raise ProtocolError("ADAPTER_FRAME_TOO_LARGE")
    return raw


def decode(raw, *, direction):
    if direction not in METHODS:
        raise ProtocolError()
    if len(raw) > MAX_FRAME_BYTES:
        raise ProtocolError("ADAPTER_FRAME_TOO_LARGE")
    if not raw.endswith(b"\n") or b"\n" in raw[:-1]:
        raise ProtocolError()
    try:
        value = json.loads(raw.decode("utf-8"), object_pairs_hook=_pairs,
                           parse_constant=_invalid_constant)
        if not isinstance(value, dict) or set(value) - {"protocol", "method", "key", "replyTo", "payload"}:
            raise ProtocolError()
        if value.get("protocol") != VERSION or not isinstance(value.get("payload"), dict):
            raise ProtocolError()
        for key in ("key", "replyTo"):
            if key == "replyTo" and key not in value:
                continue
            if not isinstance(value.get(key), str) or not re.fullmatch(r"[A-Za-z0-9._:-]{1,128}", value[key]):
                raise ProtocolError()
        if not isinstance(value.get("method"), str) or value["method"] not in METHODS[direction]:
            raise ProtocolError("ADAPTER_METHOD_UNSUPPORTED")
        _payload(value)
        from .interaction import validate_payload
        validate_payload(value["method"], value["payload"])
        from .tools import validate_payload as validate_tool_payload
        validate_tool_payload(value["method"], value["payload"])
        if value["method"] in {"message", "plan", "activity"} and len(encode(value["payload"])) > MAX_TEXT_BYTES:
            raise ProtocolError("ADAPTER_MESSAGE_TOO_LARGE")
        return value
    except (ValueError, TypeError, UnicodeError, RecursionError) as error:
        if isinstance(error, ProtocolError):
            raise
        raise ProtocolError() from None
