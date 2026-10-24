import subprocess
import sys
import unittest

from seven_agent.force.processor import ask_agent, pass_result, wait_user


class ProcessorTests(unittest.TestCase):
    def test_contracts_unchanged(self):
        self.assertEqual(pass_result()["decision"], "PASS")
        self.assertEqual(ask_agent(reason_code="CHECK_FAILED", message="repair")["decision"], "REVISE")
        self.assertEqual(wait_user(reason_code="CHECK_UNAVAILABLE", message="decide")["decision"], "WAIT_USER")

    def test_import_does_not_load_agent(self):
        result = subprocess.run([sys.executable, "-c",
            "import sys; import seven_agent.force.processor; "
            "assert 'seven_agent.force.agent' not in sys.modules"], capture_output=True, timeout=5)
        self.assertEqual(result.returncode, 0)
