import importlib.util
from pathlib import Path
import unittest
from unittest.mock import patch
import tempfile
import subprocess
import stat

spec = importlib.util.spec_from_file_location('export_schema', Path(__file__).with_name('export-schema.py'))
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)


class SchemaExportTests(unittest.TestCase):
    def test_preserves_dollar_quoted_function_body(self):
        body = "-- retained function comment\nINSERT INTO audit VALUES ('x');\n\n\nRETURN 'trailing  ';\n"
        sql = 'CREATE FUNCTION public.f() RETURNS text LANGUAGE plpgsql AS $fn$\n' + body + '$fn$;\n'
        self.assertIn(body, module.sanitize_dump(sql))

    def test_removes_dump_identity_and_pg_version_setting(self):
        sql = "-- Dumped from private host\n\\restrict random\nSET transaction_timeout = 0;\nSELECT pg_catalog.set_config('search_path', '', false);\nCREATE TABLE public.example (id bigint);\n\\unrestrict random\n"
        result = module.sanitize_dump(sql)
        self.assertNotIn('private host', result)
        self.assertNotIn('random', result)
        self.assertNotIn('transaction_timeout', result)
        self.assertIn("SET search_path = '';", result)
        self.assertIn('CREATE TABLE', result)

    def test_rejects_rows_ownership_roles_and_sequence_state(self):
        for sql in ['COPY public.example FROM stdin;', 'INSERT INTO public.example VALUES (1);',
                    'ALTER TABLE public.example OWNER TO private_owner;', 'GRANT SELECT ON public.example TO private_role;',
                    'CREATE ROLE private_role;', "SELECT pg_catalog.setval('public.example_seq', 42);"]:
            with self.subTest(sql=sql), self.assertRaises(ValueError):
                module.sanitize_dump(sql)

    def test_cli_writes_private_file_and_forces_read_only(self):
        with tempfile.TemporaryDirectory() as folder:
            output = Path(folder) / 'schema.sql'
            completed = subprocess.CompletedProcess([], 0, b'CREATE TABLE public.example (id bigint);\n', b'')
            with patch.object(module.subprocess, 'run', return_value=completed) as run, patch('sys.argv', ['export-schema', '--output', str(output)]):
                module.main()
            self.assertIn('CREATE TABLE', output.read_text())
            self.assertEqual(stat.S_IMODE(output.stat().st_mode), 0o600)
            self.assertIn('default_transaction_read_only=on', run.call_args.kwargs['env']['PGOPTIONS'])
            self.assertIn('--schema-only', run.call_args.args[0])
            with patch('sys.argv', ['export-schema', '--output', str(output)]), self.assertRaises(SystemExit):
                module.main()

    def test_public_schema_restores_into_default_empty_database(self):
        self.assertEqual(module.sanitize_dump('CREATE SCHEMA "public";\n'), 'CREATE SCHEMA IF NOT EXISTS "public";\n')

    def test_retains_schema_semantics(self):
        sql = 'CREATE TABLE public.example (id bigint PRIMARY KEY, status text CHECK (status IN (\'ACTIVE\',\'DELETED\')));\nCREATE INDEX example_status_idx ON public.example(status);\n'
        self.assertEqual(module.sanitize_dump(sql), sql)


if __name__ == '__main__':
    unittest.main()
