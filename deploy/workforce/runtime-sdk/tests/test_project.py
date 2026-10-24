import contextlib
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest

from seven_agent.force.__main__ import main


class ProjectTests(unittest.TestCase):
    def project(self, root):
        (root / 'pyproject.toml').write_text('[project]\nname="example-adapter"\nversion="1"\n[tool.setuptools.package-data]\nexample = ["data/*.json"]\n')
        package = root / 'src/example'
        (package / 'data').mkdir(parents=True)
        (package / '__init__.py').write_text('from .adapter import agent\n')
        (package / 'events.py').write_text('MESSAGE = "resource loaded"\n')
        (package / 'session.py').write_text('SESSION = "fake-session"\n')
        (package / 'data/value.json').write_text('{"value":42}')
        (package / 'adapter.py').write_text('''import json
from importlib.resources import files
from seven_agent.force.agent import AgentAdapter, RoundResult
from .events import MESSAGE
from .session import SESSION
agent = AgentAdapter(name="project", version="1")
@agent.run
async def run(request, context):
    assert json.loads(files("example").joinpath("data/value.json").read_text())["value"] == 42
    await context.started()
    await context.emit_message(MESSAGE)
    return RoundResult.finished(session_reference=SESSION)
''')
        return package

    def command(self, args):
        with contextlib.redirect_stdout(io.StringIO()), contextlib.redirect_stderr(io.StringIO()):
            return main(args)

    def test_project_inspect_dev_build_and_isolated_launch(self):
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            self.project(root)
            fixture = root / 'fixture.json'
            fixture.write_text('{}')
            for command in ('inspect', 'dev'):
                args = [command, str(root), '--module', 'example']
                if command == 'dev': args += ['--fixture', str(fixture)]
                self.assertEqual(self.command(args), 0)
            output = root / 'build'
            self.assertEqual(self.command(['build', str(root), '--module', 'example', '--output', str(output)]), 0)
            self.assertTrue((output / 'example/events.py').is_file())
            self.assertTrue((output / 'example/session.py').is_file())
            self.assertTrue((output / 'example/data/value.json').is_file())
            self.assertTrue((output / 'seven_agent/force/agent.py').is_file())
            frames = '\n'.join(json.dumps({'protocol':'seven.agent-adapter/v1', 'method':method, 'key':key, 'payload':payload}) for method,key,payload in [
                ('initialize','init',{}), ('execution.start','start',{'roundKey':'r1','inputs':{}})]) + '\n'
            # Real stdio remains open until completion; EOF cancels a running SDK round.
            process = subprocess.Popen([sys.executable, '-I', str(output/'runtime-entry.py')], stdin=subprocess.PIPE, stdout=subprocess.PIPE, stderr=subprocess.PIPE, text=True)
            try:
                process.stdin.write(frames); process.stdin.flush()
                import selectors
                import time
                deadline = time.monotonic() + 10
                received = []
                # Binary unbuffered reads avoid text-buffer/select mismatches.
                pending = b''
                with selectors.DefaultSelector() as selector:
                    selector.register(process.stdout, selectors.EVENT_READ)
                    while not any(f['method']=='execution.finished' for f in received):
                        if time.monotonic() >= deadline: self.fail('adapter did not finish')
                        for key, _ in selector.select(0.2):
                            chunk = os.read(key.fd, 65536)
                            if not chunk: self.fail('adapter exited before result')
                            pending += chunk
                            while b'\n' in pending:
                                line, pending = pending.split(b'\n',1)
                                received.append(json.loads(line))
                result = next(f for f in received if f['method']=='execution.finished')
                self.assertEqual(result['payload']['outcome'], 'FINISHED')
                self.assertEqual(result['payload']['sessionReference'], 'fake-session')
            finally:
                process.stdin.close()
                try: process.wait(timeout=7)
                except subprocess.TimeoutExpired: process.kill(); process.wait()
                process.stdout.close(); process.stderr.close()
            self.assertEqual(process.returncode, 0)

    def test_project_rejects_missing_resource_symlink_and_secret(self):
        for scenario in ('missing', 'symlink', 'secret'):
            with self.subTest(scenario=scenario), tempfile.TemporaryDirectory() as directory:
                root = Path(directory); package = self.project(root)
                if scenario == 'missing': (package/'data/value.json').unlink()
                elif scenario == 'symlink': (package/'escape').symlink_to(root, target_is_directory=True)
                else: (package/'.env').write_text('do-not-package')
                self.assertEqual(self.command(['build',str(root),'--module','example','--output',str(root/'out')]),1)
                self.assertFalse((root/'out').exists())

    def test_separate_projects_do_not_reuse_import_cache(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory)
            for name in ('one','two'):
                project=root/name;project.mkdir();package=self.project(project)
                if name=='two': (package/'events.py').write_text('raise RuntimeError("not cached")')
                self.assertEqual(self.command(['inspect',str(project),'--module','example']),0 if name=='one' else 1)

    def test_flat_project_build_is_deterministic_and_never_overwrites(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);package=self.project(root)
            package.rename(root/'example')
            for name in ('first','second'):
                self.assertEqual(self.command(['build',str(root),'--module','example','--output',str(root/name)]),0)
            self.assertEqual((root/'first/source-index.json').read_bytes(),(root/'second/source-index.json').read_bytes())
            self.assertEqual(self.command(['build',str(root),'--module','example','--output',str(root/'first')]),1)

    def test_rejects_ambiguous_layout_and_resource_traversal(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);self.project(root)
            (root/'example').mkdir();(root/'example/__init__.py').write_text('')
            self.assertEqual(self.command(['inspect',str(root),'--module','example']),1)
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);self.project(root)
            (root/'pyproject.toml').write_text('[project]\nname="example"\n[tool.setuptools.package-data]\nexample=["../secret"]')
            self.assertEqual(self.command(['build',str(root),'--module','example','--output',str(root/'out')]),1)

    def test_rejects_stdlib_shadowing_and_nested_output(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory);package=self.project(root)
            self.assertEqual(self.command(['build',str(root),'--module','example','--output',str(package/'built')]),1)
            self.assertFalse((package/'built').exists())
            package.rename(root/'src/json')
            self.assertEqual(self.command(['inspect',str(root),'--module','json']),1)
