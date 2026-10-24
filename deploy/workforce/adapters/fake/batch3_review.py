"""Fixed local Batch3 result Reviewer; no Provider or instruction-selected verdict."""
import hashlib
import json
from pathlib import Path
import sys

sys.path[:0] = ["/opt/seven-worker/result-runtime", "/opt/seven/adapter"]

from adapter import directory, read, regular, write, path, require, validate, FakeFailure
from seven_agent.force.agent import AgentAdapter, ActivityStatus, RoundResult

FIXTURE_SHA256 = "b96229fa352478a47a0d95f5fa7550c6564b42db1eebb08cb30556ffe5795247"
asset = Path(__file__).absolute().parent / "fixtures" / "batch3-review.json"
with directory(asset.parent) as fd:
    raw = read(fd, asset.name)
require(hashlib.sha256(raw).hexdigest() == FIXTURE_SHA256)
require(json.loads(raw) == {"role": "REVIEW", "scenario": "BATCH3_RESULT"})
adapter = AgentAdapter(name="fake", version="batch3-review")


def candidate_report(workspace):
    checks = []
    with directory(path(workspace)) as repo:
        for name in ("batch3-result.txt", "batch3-supplements.txt"):
            try:
                content = read(repo, name) if regular(repo, name, missing=True) is not None else b""
                matched = content in (b"Fixed answer: alpha\n", b"Fixed answer: beta\n") if name == "batch3-result.txt" else bool(content.decode("utf-8").strip())
            except (FakeFailure, OSError, UnicodeError):
                matched = False
            checks.append({"command": "inspect fixed " + name, "exitCode": 0 if matched else 1,
                           "status": "PASSED" if matched else "FAILED",
                           "summary": "Fixed Batch3 file check passed." if matched else "Fixed Batch3 file is missing or invalid."})
    passed = all(check["exitCode"] == 0 for check in checks)
    return {"schemaVersion": "review-report/v1", "verdict": "PASS" if passed else "REVISE",
            "score": 100 if passed else 60, "summary": "Fixed Batch3 result checks passed." if passed else "Correct the Batch3 answer or supplement file.",
            "reviewTests": checks}


@adapter.run
async def run(request, context):
    validate(request.inputs, "REVIEW")
    await context.started()
    await context.emit_activity(key="fake:batch3-review", title="Inspect fixed Batch3 result files", status=ActivityStatus.STARTED)
    report = candidate_report(request.inputs["workspace"])
    output = path(request.inputs["output_path"])
    with directory(output.parent) as fd:
        write(fd, output.name, json.dumps(report, sort_keys=True, separators=(",", ":")).encode())
    await context.emit_activity(key="fake:batch3-review", title="Inspect fixed Batch3 result files", status=ActivityStatus.COMPLETED)
    return RoundResult.finished(summary="Fixed Batch3 candidate review report recorded")


if __name__ == "__main__":
    adapter.serve()
