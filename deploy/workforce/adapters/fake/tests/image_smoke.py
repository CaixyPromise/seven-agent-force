"""Run via python3 -I - inside a dedicated, network-none Batch3 image container."""
import asyncio
import hashlib
import json
import os
from pathlib import Path
import shutil


async def main():
    manifest = json.loads(Path("/opt/seven-worker/contract/manifest.json").read_text())
    assert os.getuid() == 10001
    assert manifest["executionKinds"] == ["AGENT_ATTEMPT"]
    index_path = Path("/opt/seven/adapter/source-index.json")
    assert hashlib.sha256(index_path.read_bytes()).hexdigest() == manifest["adapter"]["sourceSha256"]
    entries = {entry["path"]: entry["sha256"] for entry in json.loads(index_path.read_text())}
    for name in ("entrypoint", "batch3.py", "interaction.py", "adapter.py", "fixtures/batch3.json"):
        assert hashlib.sha256((index_path.parent / name).read_bytes()).hexdigest() == entries[name]
    processes = []
    events = []

    async def send(process, method, key, payload):
        process.stdin.write((json.dumps(dict(protocol="seven.agent-adapter/v1", method=method, key=key, payload=payload)) + "\n").encode())
        await process.stdin.drain()

    async def until(process, method):
        while True:
            raw = await asyncio.wait_for(process.stdout.readline(), 5)
            assert raw, "unexpected adapter EOF"
            frame = json.loads(raw)
            events.append(frame)
            if frame["method"] == method:
                return frame
            assert frame["method"] != "error", frame["payload"].get("code", "ADAPTER_ERROR")

    async def start():
        process = await asyncio.create_subprocess_exec("/opt/seven/adapter/entrypoint",
            stdin=asyncio.subprocess.PIPE, stdout=asyncio.subprocess.PIPE, stderr=asyncio.subprocess.PIPE)
        processes.append(process)
        await send(process, "initialize", "init", {})
        ready = (await until(process, "ready"))["payload"]
        assert ready["name"] == manifest["adapter"]["id"]
        assert ready["version"] == manifest["adapter"]["version"]
        assert ready["capabilities"] == manifest["adapter"]["capabilities"]
        return process

    def inputs(attempt):
        root = Path("/workspace/123/attempts") / attempt
        for name in ("repo", "state", "output", "tmp"):
            (root / name).mkdir(parents=True, mode=0o700)
        return dict(mode="AGENT", taskId="123", attemptId=attempt,
                    instruction="Fixed image smoke", workspace=str(root / "repo"),
                    state=str(root / "state"), output=str(root / "output"), temporary=str(root / "tmp"),
                    provider={"model": "ignored-no-model-call"}, feedback=None)

    try:
        original = inputs("456")
        first = await start()
        await send(first, "execution.start", "run", {"roundKey": "r1", "inputs": original})
        question = await until(first, "interaction.request")
        assert [f["method"] for f in events if f["method"] != "ready"] == [
            "execution.started", "activity", "activity", "message", "plan", "interaction.request"]
        assert [f["payload"]["status"] for f in events if f["method"] == "activity"] == ["STARTED", "COMPLETED"]
        assert question["payload"]["questions"][0]["kind"] == "SINGLE_CHOICE"
        supplement = {"roundKey": "r1", "messageKey": "supplement:1", "text": "Preserve this exact supplement."}
        await send(first, "user.message", "message:1", supplement)
        assert (await until(first, "message.received"))["payload"]["status"] == "RECEIVED"
        await send(first, "user.message", "message:retry", supplement)
        assert (await until(first, "message.received"))["payload"]["status"] == "RECEIVED"
        await send(first, "execution.suspend", "suspend:1", {"roundKey": "r1"})
        reference = (await until(first, "execution.suspended"))["payload"]["sessionReference"]
        state_path = Path(original["state"]) / "codex/fake-session.json"
        state = json.loads(state_path.read_text())
        assert state["phase"] == "SUSPENDED" and state["answer"] is None
        assert len(state["supplements"]) == 1 and state["sessionId"] == reference
        assert not Path(original["workspace"], "batch3-result.txt").exists()
        assert not any(frame["method"] == "execution.finished" for frame in events)
        assert not any(f["method"] == "activity" and f["payload"]["status"] == "SUCCEEDED" for f in events)
        first.stdin.close()
        await asyncio.wait_for(first.wait(), 5)
        assert first.returncode == 0 and await first.stderr.read() == b""
        restored = inputs("789")
        shutil.copytree(Path(original["state"]) / "codex", Path(restored["state"]) / "codex")
        restored["interactionResponse"] = {"interactionKey": "fake:question", "responseId": "101", "answers": [
            {"questionKey": "result", "selectedOptionKeys": ["beta"], "text": ""}]}
        restored["interactionQuestionRequest"] = json.loads(Path(restored["state"], "codex/fake-session.json").read_text())["question"]
        second = await start()
        boundary = len(events)
        await send(second, "execution.resume", "resume", {"roundKey": "r2", "sessionReference": reference, "inputs": restored})
        await until(second, "execution.started")
        assert not Path(restored["workspace"], "batch3-result.txt").exists()
        assert json.loads(Path(restored["state"], "codex/fake-session.json").read_text())["answer"] is None
        await send(second, "interaction.respond", "cold-response", {"roundKey": "r2", **restored["interactionResponse"]})
        receipt = await until(second, "interaction.received")
        assert receipt["replyTo"] == "cold-response"
        assert receipt["payload"] == {"roundKey": "r2", "interactionKey": "fake:question", "responseId": "101", "status": "RECEIVED"}
        result = (await until(second, "execution.finished"))["payload"]
        assert [f["payload"]["status"] for f in events[boundary:] if f["method"] == "activity"] == [
            "STARTED", "COMPLETED", "STARTED", "SUCCEEDED"]
        assert result["outcome"] == "FINISHED" and result["sessionReference"] == reference
        assert not any(frame["method"] == "interaction.request" for frame in events[boundary:])
        assert Path(restored["workspace"], "batch3-result.txt").read_text() == "Fixed answer: beta\n"
        assert Path(restored["workspace"], "batch3-supplements.txt").read_text() == "Preserve this exact supplement.\n"
        final = json.loads(Path(restored["state"], "codex/fake-session.json").read_text())
        assert final["sessionId"] == reference and final["round"] == 2
        assert final["answer"] == restored["interactionResponse"]
        saved_bytes = Path(restored["state"], "codex/fake-session.json").read_bytes()
        result_mtime = Path(restored["workspace"], "batch3-result.txt").stat().st_mtime_ns
        await send(second, "interaction.respond", "cold-response-retry", {"roundKey": "r2", **restored["interactionResponse"]})
        replay = await until(second, "interaction.received")
        assert replay["replyTo"] == "cold-response-retry" and replay["payload"] == receipt["payload"]
        assert Path(restored["state"], "codex/fake-session.json").read_bytes() == saved_bytes
        assert Path(restored["workspace"], "batch3-result.txt").stat().st_mtime_ns == result_mtime
        print(json.dumps({"status": "PASS", "uid": os.getuid(), "adapter": "fake", "version": "batch3",
            "capabilitiesMatch": True, "indexedModulesAndResources": True, "supplementSafePoint": True,
            "suspendBeforeAnswer": True, "originalProcessExited": True, "freshProcessNewAttemptResume": True,
            "sameSession": True, "savedAnswerConsumed": True, "questionReissued": False,
            "normalColdReceiptRequired": True, "coldReceiptReplyToVerified": True,
            "duplicateColdAnswerNoMutation": True,
            "realPreparationObservations": True, "recordSuccessAfterPersistence": True,
            "providerCalls": 0}, sort_keys=True))
    finally:
        for process in processes:
            if process.returncode is None:
                process.stdin.close()
                try:
                    await asyncio.wait_for(process.wait(), 5)
                except asyncio.TimeoutError:
                    process.kill()
                    await process.wait()
            assert process.returncode == 0
            assert await process.stderr.read() == b""


asyncio.run(main())
