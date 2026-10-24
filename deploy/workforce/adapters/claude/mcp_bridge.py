"""Narrow Claude Code MCP bridge for platform-owned tools and user questions.

Claude starts this process over stdio.  The bridge never receives provider
credentials: it forwards bounded JSON-RPC tool calls to the image adapter over
a private Unix socket owned by the current Attempt.
"""
from __future__ import annotations

import asyncio
import json
import os
import sys


MAX_LINE = 128 << 10


def _reply(identifier, *, result=None, error=None):
    value = {"jsonrpc": "2.0", "id": identifier}
    if error is not None:
        value["error"] = error
    else:
        value["result"] = result
    sys.stdout.write(json.dumps(value, separators=(",", ":"), ensure_ascii=False) + "\n")
    sys.stdout.flush()


async def _forward(native_call_id, name, arguments):
    socket_path = os.environ.get("SEVEN_CLAUDE_MCP_SOCKET", "")
    if not socket_path.startswith("/"):
        raise ValueError("CLAUDE_MCP_SOCKET_INVALID")
    reader, writer = await asyncio.open_unix_connection(socket_path)
    try:
        body = json.dumps({"nativeCallId": str(native_call_id), "name": name, "arguments": arguments}, separators=(",", ":"),
                          ensure_ascii=False).encode("utf-8")
        if len(body) > MAX_LINE:
            raise ValueError("CLAUDE_MCP_REQUEST_TOO_LARGE")
        writer.write(body + b"\n")
        await writer.drain()
        raw = await reader.readline()
        if not raw or len(raw) > MAX_LINE:
            raise ValueError("CLAUDE_MCP_RESPONSE_INVALID")
        value = json.loads(raw)
        if not isinstance(value, dict) or set(value) != {"ok", "value"}:
            raise ValueError("CLAUDE_MCP_RESPONSE_INVALID")
        if not value["ok"]:
            raise ValueError(str(value["value"]))
        return value["value"]
    finally:
        writer.close()
        await writer.wait_closed()


def _catalog():
    path = os.environ.get("SEVEN_CLAUDE_MCP_CATALOG", "")
    with open(path, "r", encoding="utf-8") as stream:
        value = json.load(stream)
    if not isinstance(value, list) or len(value) > 17:
        raise ValueError("CLAUDE_MCP_CATALOG_INVALID")
    return value


async def main():
    catalog = _catalog()
    while True:
        request = {}
        raw = await asyncio.to_thread(sys.stdin.buffer.readline, MAX_LINE + 1)
        if not raw:
            return
        if len(raw) > MAX_LINE:
            return
        try:
            request = json.loads(raw)
            identifier = request.get("id")
            method = request.get("method")
            if method == "initialize":
                _reply(identifier, result={
                    "protocolVersion": "2025-06-18",
                    "capabilities": {"tools": {"listChanged": False}},
                    "serverInfo": {"name": "seven-platform", "version": "1"},
                })
            elif method == "notifications/initialized":
                continue
            elif method == "tools/list":
                _reply(identifier, result={"tools": catalog})
            elif method == "tools/call":
                params = request.get("params") or {}
                value = await _forward(identifier, params.get("name"), params.get("arguments") or {})
                text = json.dumps(value, separators=(",", ":"), ensure_ascii=False)
                _reply(identifier, result={"content": [{"type": "text", "text": text}],
                                           "structuredContent": value, "isError": False})
            elif identifier is not None:
                _reply(identifier, error={"code": -32601, "message": "Method not found"})
        except Exception as error:
            if isinstance(request, dict) and request.get("id") is not None:
                _reply(request["id"], error={"code": -32000, "message": str(error)[:256]})


if __name__ == "__main__":
    asyncio.run(main())
