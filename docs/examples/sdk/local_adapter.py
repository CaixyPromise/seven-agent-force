"""In-process SDK example: no Provider, platform API, Docker or Git access."""
from seven_agent.force.agent import AgentAdapter, RoundResult

agent = AgentAdapter(name="local-example", version="1")

@agent.run
async def run(request, context):
    # Synthetic in-process execution; a native adapter must spawn its process first.
    await context.started()
    await context.emit_message("Synthetic local SDK example completed.")
    return RoundResult.finished(
        summary="Local example only", session_reference="synthetic-local-session"
    )

if __name__ == "__main__":
    agent.serve()
