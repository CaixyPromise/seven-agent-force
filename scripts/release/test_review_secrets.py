import importlib.util
from pathlib import Path
import subprocess
import tempfile
import unittest

spec = importlib.util.spec_from_file_location('review', Path(__file__).with_name('review_secrets.py'))
review = importlib.util.module_from_spec(spec)
spec.loader.exec_module(review)

class ExactFindingTests(unittest.TestCase):
    def setUp(self):
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.git('init', '-q')
        self.file = self.root / 'fixture.txt'
        self.file.write_text('dummy fixture\n')
        self.commit()
        self.finding = dict(File='fixture.txt', StartLine=1, EndLine=1, RuleID='test-rule')
        self.item = review.identity(self.finding, 'source', self.root)
        self.item.update(decision='false_positive', reason='Synthetic test')

    def git(self, *args):
        return subprocess.check_output(['git', '-C', str(self.root), *args])

    def commit(self):
        self.git('add', 'fixture.txt')
        self.git('-c', 'user.name=Test', '-c', 'user.email=test@example.invalid', 'commit', '-qm', 'fixture')

    def test_exact_match_only(self):
        self.assertFalse(review.adjudicate([self.finding], [self.item], 'source', self.root)['unresolved'])
        changed = dict(self.finding, RuleID='another-rule')
        self.assertTrue(review.adjudicate([changed], [self.item], 'source', self.root)['unresolved'])

    def test_replaced_content_rejected(self):
        self.file.write_text('different potentially sensitive value\n')
        self.commit()
        self.assertTrue(review.adjudicate([self.finding], [self.item], 'source', self.root)['unresolved'])

    def test_new_path_not_allowed(self):
        (self.root / 'other.txt').write_text('dummy fixture\n')
        self.git('add', 'other.txt')
        self.git('-c', 'user.name=Test', '-c', 'user.email=test@example.invalid', 'commit', '-qm', 'other')
        self.assertTrue(review.adjudicate([dict(self.finding, File='other.txt')], [self.item], 'source', self.root)['unresolved'])

    def test_history_requires_exact_commit(self):
        old = self.git('rev-parse', 'HEAD').decode().strip()
        f = dict(self.finding, Commit=old)
        item = review.identity(f, 'history', self.root)
        item.update(decision='false_positive', reason='Synthetic test')
        self.assertFalse(review.adjudicate([f], [item], 'history', self.root)['unresolved'])
        self.file.write_text('dummy fixture\nnew line\n')
        self.commit()
        f['Commit'] = self.git('rev-parse', 'HEAD').decode().strip()
        self.assertTrue(review.adjudicate([f], [item], 'history', self.root)['unresolved'])

    def test_missing_blob_and_invalid_line_fail(self):
        with self.assertRaises(Exception):
            review.identity(dict(self.finding, File='missing'), 'source', self.root)
        with self.assertRaises(ValueError):
            review.identity(dict(self.finding, EndLine=999), 'source', self.root)

if __name__ == '__main__':
    unittest.main()
