import copy
import unittest
from verified_checks import validate, REPO


class VerifiedChecksTest(unittest.TestCase):
    def setUp(self):
        self.run = {'repository': {'full_name': REPO}, 'event': 'push',
                    'path': '.github/workflows/ci.yml', 'status': 'completed', 'conclusion': 'success'}
        self.jobs = [{'name': n, 'status': 'completed', 'conclusion': 'success'} for n in ['server', 'web', 'secrets']]

    def test_same_commit_and_release_only_changes(self):
        validate(self.run, self.jobs, [])
        validate(self.run, self.jobs, ['scripts/release/build.sh', 'docs/deployment.md'])

    def test_application_dependency_and_workflow_changes_rejected(self):
        for path in ['seven-agent-force-server/go.sum', 'seven-agent-force-web/pnpm-lock.yaml',
                     'seven-agent-force-server/internal/example.go', '.github/workflows/ci.yml']:
            with self.subTest(path=path), self.assertRaises(ValueError):
                validate(self.run, self.jobs, [path])

    def test_failed_skipped_missing_duplicate_jobs_rejected(self):
        for jobs in [self.jobs[:-1], self.jobs + [self.jobs[0]]]:
            with self.assertRaises(ValueError): validate(self.run, jobs, [])
        for verdict in ['failure', 'skipped', None]:
            jobs = copy.deepcopy(self.jobs); jobs[0]['conclusion'] = verdict
            with self.assertRaises(ValueError): validate(self.run, jobs, [])

    def test_untrusted_or_pending_workflow_rejected(self):
        for key, value in [('repository', {'full_name': 'other/repo'}), ('event', 'pull_request'),
                           ('path', '.github/workflows/other.yml'), ('status', 'in_progress'), ('conclusion', 'failure')]:
            run = copy.deepcopy(self.run); run[key] = value
            with self.assertRaises(ValueError): validate(run, self.jobs, [])
