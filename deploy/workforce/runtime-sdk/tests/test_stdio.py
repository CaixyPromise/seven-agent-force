"""Real pipe smoke with a deterministic in-process fake, never a Provider."""
import json
import os
import subprocess
import sys
import unittest

from seven_agent.force._protocol import VERSION, encode


class StdioTests(unittest.TestCase):
    def test_handshake_over_real_stdio(self):
        program = '''
from seven_agent.force.agent import AgentAdapter, RoundResult
agent = AgentAdapter(name="pipe-fixture", version="1")
@agent.run
async def run(request, context):
    await context.started()
    return RoundResult.finished()
agent.serve()
'''
        result = subprocess.run([sys.executable, "-c", program],
                                input=encode(dict(protocol=VERSION, method="initialize", key="init", payload={})),
                                capture_output=True, timeout=5, env=os.environ.copy())
        self.assertEqual(result.returncode, 0, result.stderr.decode())
        frame = json.loads(result.stdout)
        self.assertEqual(frame["method"], "ready")
        self.assertEqual(frame["replyTo"], "init")
        self.assertEqual(result.stderr, b"")
