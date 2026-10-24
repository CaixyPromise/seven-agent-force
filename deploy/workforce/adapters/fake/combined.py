"""Fixed Batch1 image entrypoint supporting both standard execution kinds."""
import sys

# The production launcher uses python3 -I: neither script directory nor
# PYTHONPATH is an import source. Only these protected image paths are added.
sys.path[:0] = ["/opt/seven-worker/result-runtime", "/opt/seven/adapter"]

from seven_agent.force.agent import AgentAdapter
from adapter import build_execution, require

_, _agent = build_execution("batch1.json", "0174ee3ce514c1bf06daa0dc4ac88ea372913ea8f707e3c5764b57fd9f04b7af")
_, _review = build_execution("batch1-review.json", "220b962f93f078010b6656caab7a70ae37788c87e44224563a405ac088cd1e27")
adapter = AgentAdapter(name="fake", version="batch2.1")


@adapter.run
async def run(request, context):
    require(isinstance(request.inputs, dict))
    mode = request.inputs.get("mode")
    require(mode in ("AGENT", "REVIEW"))
    execute = _agent if mode == "AGENT" else _review
    return await execute(request, context)


@adapter.resume
async def resume(request, context):
    require(isinstance(request.inputs, dict) and request.inputs.get("mode") == "AGENT")
    return await _agent(request, context, True)


if __name__ == "__main__":
    adapter.serve()
