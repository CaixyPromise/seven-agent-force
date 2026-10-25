"""Opt-in destructive tests restricted to a disposable PostgreSQL database.
Create force_storage_contract_test_regression in a dedicated instance, supply
libpq PG* settings and set FORCE_STORAGE_CONTRACT_TEST=1. Never use a live DB.
"""
import os
from pathlib import Path
import re
import subprocess
import unittest

ROOT = Path(__file__).resolve().parents[2]
SQL = (ROOT / 'seven-agent-force-server/internal/app/workforce/schemacontract/columns.sql').read_text()
TARGETS = list(dict.fromkeys(re.findall(r"\('([a-z_0-9]+)','([a-z_0-9]+)','([^']+)',(\d+),(true|false)\)", SQL)))


@unittest.skipUnless(os.environ.get('FORCE_STORAGE_CONTRACT_TEST') == '1', 'dedicated PostgreSQL opt-in required')
class StorageContractPostgresTests(unittest.TestCase):
    def execute(self, sql, success=True):
        result = subprocess.run(['psql', '-XAt', '-v', 'ON_ERROR_STOP=1'], input=sql,
                                capture_output=True, text=True)
        # Do not print connection errors or fixture contents on failure.
        self.assertEqual(result.returncode == 0, success, 'dedicated storage contract SQL outcome mismatch')
        return result

    def setUp(self):
        if os.environ.get('PGDATABASE') != 'force_storage_contract_test_regression':
            self.fail('PGDATABASE must be exactly force_storage_contract_test_regression')
        self.assertEqual(self.execute('SELECT current_database();').stdout.strip(), 'force_storage_contract_test_regression')
        self.assertEqual(len(TARGETS), 280)
        self.execute('DROP SCHEMA public CASCADE; CREATE SCHEMA public;')
        groups = {}
        for table, column, target, limit, required in TARGETS:
            # Model the historical TEXT/wide enum storage. Source frozen fields
            # are nullable; other required fields have synthetic harmless defaults.
            source = 'bigint' if column == 'revision' else 'integer' if target in ('smallint', 'integer') else 'text'
            nullable = table == 'workforce_task_assignments' and column in ('model_protocol', 'credential_version')
            suffix = '' if required == 'false' or nullable else " NOT NULL DEFAULT " + ('1' if source in ('bigint', 'integer') else "'x'")
            groups.setdefault(table, []).append(f'"{column}" {source}{suffix}')
        self.execute('\n'.join(f'CREATE TABLE "{table}" ({",".join(columns)});' for table, columns in groups.items()))

    def reject(self, fixture, expected):
        result = self.execute('BEGIN;\n' + fixture + '\n' + SQL + '\nCOMMIT;', success=False)
        self.assertIn(expected, result.stderr)
        # The entire transaction, including fixtures, must roll back.
        self.assertEqual(self.execute('SELECT count(*) FROM workforce_tasks;').stdout.strip(), '0')
        self.assertEqual(self.execute("SELECT data_type FROM information_schema.columns WHERE table_name='workforce_tasks' AND column_name='idempotency_key';").stdout.strip(), 'text')

    def test_oversized_key_rejected_without_partial_changes(self):
        self.reject("INSERT INTO workforce_tasks(idempotency_key) VALUES(repeat('x',129));", 'SCHEMA_CONTRACT_LENGTH')

    def test_http_integer_overflow_rejected(self):
        self.reject('INSERT INTO workforce_webhook_delivery_attempts(http_status) VALUES(40000);', 'SCHEMA_CONTRACT_INTEGER_RANGE')

    def test_missing_frozen_input_is_not_fabricated(self):
        self.reject("INSERT INTO workforce_task_assignments(model_protocol,credential_version) VALUES(NULL,1);", 'SCHEMA_CONTRACT_FROZEN_INPUT_REQUIRED')

    def test_boundaries_unbounded_content_and_repeatability(self):
        self.execute("INSERT INTO workforce_tasks(idempotency_key,repository_url) VALUES(repeat('x',128),'https://example.invalid/'||repeat('x',2200)); INSERT INTO workforce_artifacts(name) VALUES(repeat('文',2000)); INSERT INTO workforce_hook_package_revisions(revision) VALUES(2147483648);")
        self.execute(SQL)
        self.execute(SQL)
        self.assertEqual(self.execute('SELECT char_length(repository_url) FROM workforce_tasks;').stdout.strip(), '2224')
        self.assertEqual(self.execute('SELECT char_length(name) FROM workforce_artifacts;').stdout.strip(), '2000')
        self.assertEqual(self.execute('SELECT revision FROM workforce_hook_package_revisions;').stdout.strip(), '2147483648')


    def test_legacy_validation_guards_reject_invalid_values(self):
        self.execute(SQL)
        self.execute("""
ALTER TABLE workforce_harness_releases ADD COLUMN manifest_json jsonb DEFAULT '{}', ADD COLUMN provider_protocols_json jsonb DEFAULT '[]', ADD COLUMN hook_events_json jsonb DEFAULT '[]';
CREATE TABLE workforce_model_credentials(secret_version integer DEFAULT 1, version integer DEFAULT 1, models_json jsonb DEFAULT '[]', tenant_id bigint, status text, name text);
CREATE TABLE workforce_model_invocations(reasoning_output_tokens bigint, credential_version integer DEFAULT 1, request_body_sha256 bytea DEFAULT decode(repeat('00',32),'hex'), nonce_sha256 bytea DEFAULT decode(repeat('00',32),'hex'), upstream_status_code integer, duration_milliseconds integer, input_tokens bigint, cached_input_tokens bigint, cache_creation_input_tokens bigint, output_tokens bigint, total_tokens bigint, tenant_id bigint, model_credential_id bigint, started_at timestamptz, task_id bigint);
ALTER TABLE workforce_task_assignments ADD COLUMN hook_configuration_digest text DEFAULT '';
ALTER TABLE workforce_tasks ADD COLUMN model_credential_id bigint, ADD COLUMN primary_model text, ADD COLUMN selected_runtime_release_id bigint;
ALTER TABLE workforce_review_run_attempts ADD COLUMN credential_version integer DEFAULT 1, ADD COLUMN model_protocol text DEFAULT 'OPENAI';
""")
        guards = (ROOT / 'seven-agent-force-server/internal/app/workforce/schemacontract/guards.sql').read_text()
        self.execute(guards)
        self.execute(guards)
        for fixture in [
            'INSERT INTO workforce_model_invocations(input_tokens) VALUES(-1);',
            "INSERT INTO workforce_task_assignments(credential_version,model_protocol) VALUES(1,'BOGUS');",
            "INSERT INTO workforce_model_credentials(models_json) VALUES('{}');",
        ]:
            self.execute('BEGIN; ' + fixture + ' COMMIT;', success=False)
        self.assertEqual(self.execute('SELECT count(*) FROM workforce_model_invocations;').stdout.strip(), '0')


if __name__ == '__main__':
    unittest.main()
