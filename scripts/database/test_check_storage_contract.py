import importlib.util
from pathlib import Path
import subprocess
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location('preflight', Path(__file__).with_name('check-storage-contract.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


def zero_counts(sql):
    return '|'.join('0' for _ in range(max(1, sql.count('FILTER'))))


class StoragePreflightTests(unittest.TestCase):
    def test_covers_reviewed_columns_and_guards(self):
        with patch.object(module, 'query', side_effect=zero_counts):
            result = module.check()
        self.assertEqual((result['column_targets'], result['guard_targets']), (280, 25))
        self.assertTrue(result['ready'])
        self.assertEqual(result['issues'], [])

    def test_reports_only_counts_and_column_names(self):
        def counts(sql):
            if 'FROM public."workforce_tasks"' in sql and 'char_length("idempotency_key"' in sql:
                return '2|0'
            return zero_counts(sql)
        with patch.object(module, 'query', side_effect=counts):
            result = module.check()
        self.assertFalse(result['ready'])
        self.assertEqual(result['issues'], [{'column': 'workforce_tasks.idempotency_key', 'reason': 'LENGTH', 'count': 2}])

    def test_forces_read_only_and_withholds_connection_errors(self):
        failure = subprocess.CompletedProcess([], 1, '', 'private connection details')
        with patch.object(module.subprocess, 'run', return_value=failure) as call:
            with self.assertRaises(SystemExit) as error:
                module.query('SELECT 1')
        self.assertNotIn('private connection details', str(error.exception))
        self.assertIn('default_transaction_read_only=on', call.call_args.kwargs['env']['PGOPTIONS'])
