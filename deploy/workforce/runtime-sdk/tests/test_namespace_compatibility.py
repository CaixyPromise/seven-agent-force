import importlib
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from seven_agent.force._project import build_project


class NamespaceCompatibilityTests(unittest.TestCase):
    def test_legacy_exports_have_identical_types(self):
        legacy = importlib.import_module('seven_workforce_runtime')
        for name in ('agent', 'interaction', 'tools', '_protocol', 'processor'):
            self.assertIs(importlib.import_module('seven_workforce_runtime.' + name),
                          importlib.import_module('seven_agent.force.' + name))
            self.assertIs(getattr(legacy, name), importlib.import_module('seven_agent.force.' + name))

    def test_legacy_cli_preserves_failure_exit_code(self):
        src = Path(__file__).resolve().parents[1] / 'src'
        with tempfile.TemporaryDirectory() as directory:
            for namespace in ('seven_agent.force', 'seven_workforce_runtime'):
                script = 'import sys, runpy; sys.path.insert(0, sys.argv.pop(1)); runpy.run_module(sys.argv.pop(1), run_name="__main__")'
                result = subprocess.run([sys.executable, '-I', '-B', '-c', script, str(src), namespace,
                                         'inspect', str(Path(directory) / 'missing.py')],
                                        capture_output=True, text=True, timeout=10)
                self.assertEqual(result.returncode, 1, result.stderr)

    def test_isolated_bundle_supports_legacy_adapter_imports(self):
        source = b'from seven_workforce_runtime.agent import AgentAdapter\nadapter = AgentAdapter(name="legacy", version="1")\n'
        with tempfile.TemporaryDirectory() as directory:
            output = Path(directory) / 'bundle'
            build_project(output, 'example', {'example/__init__.py': source}, {})
            script = (
                'import sys; sys.path.insert(0, sys.argv[1]); '
                'import example; '
                'from seven_agent.force.agent import AgentAdapter; '
                'assert isinstance(example.adapter, AgentAdapter)'
            )
            result = subprocess.run([sys.executable, '-I', '-B', '-c', script, str(output)],
                                    capture_output=True, text=True, timeout=10)
            self.assertEqual(result.returncode, 0, result.stderr)
