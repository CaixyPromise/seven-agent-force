import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest

spec=importlib.util.spec_from_file_location('history',Path(__file__).with_name('check_history_files.py'))
history=importlib.util.module_from_spec(spec);spec.loader.exec_module(history)

class SnapshotHistoryTests(unittest.TestCase):
    def test_private_snapshot_tree_is_found_without_reading_content(self):
        with tempfile.TemporaryDirectory() as directory:
            root=Path(directory)
            def git(*args):return subprocess.check_output(['git','-C',directory,*args])
            git('init','-q');(root/'public.txt').write_text('public')
            git('add','.');git('-c','user.name=Test','-c','user.email=test@example.invalid','commit','-qm','public')
            public=git('rev-parse','HEAD').decode().strip()
            branch=git('symbolic-ref','HEAD').decode().strip()
            (root/'admin-pwd.txt').touch();(root/'.env.private').touch()
            git('add','.');git('-c','user.name=Test','-c','user.email=test@example.invalid','commit','-qm','private snapshot fixture')
            private=git('rev-parse','HEAD').decode().strip();git('update-ref','refs/codex/test-fixture',git('rev-parse',private+'^{tree}').decode().strip())
            git('checkout','--detach',public)
            git('update-ref',branch,public)
            self.assertEqual(history.paths(root,'HEAD'),[])
            self.assertEqual(history.paths(root,'--all'),['.env.private','admin-pwd.txt'])

if __name__=='__main__':unittest.main()
