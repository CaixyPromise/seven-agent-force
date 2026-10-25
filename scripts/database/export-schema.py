#!/usr/bin/env python3
"""Export public PostgreSQL DDL without rows, owners, ACLs or client metadata.
Connection is supplied only through libpq environment/service configuration.
"""
import argparse
import os
from pathlib import Path
import re
import subprocess


def sanitize_dump(raw):
    lines = []
    top_level = []
    body_tag = None
    for line in raw.splitlines():
        if body_tag is not None:
            lines.append(line)
            if line.count(body_tag) % 2:
                body_tag = None
            continue
        match = re.search(r'\$(?:[A-Za-z_][A-Za-z_0-9]*)?\$', line)
        if match and line.count(match.group()) % 2:
            body_tag = match.group()
        if line.startswith('--') or line.startswith('\\restrict ') or line.startswith('\\unrestrict '):
            continue
        if line == 'CREATE SCHEMA "public";':
            line = 'CREATE SCHEMA IF NOT EXISTS "public";'
        if line == 'SET transaction_timeout = 0;':
            continue  # pg_dump 17+ emits this even when dumping a PG16 server.
        if line.startswith("SELECT pg_catalog.set_config('search_path', '', false);"):
            line = "SET search_path = '';"
        lines.append(line)
        top_level.append(line)
    result = '\n'.join(lines).strip() + '\n'
    commands = '\n'.join(top_level)
    if re.search(r'^(?:COPY |INSERT INTO |GRANT |REVOKE |ALTER .* OWNER TO |CREATE DATABASE |ALTER DATABASE |CREATE ROLE |ALTER ROLE )', commands, re.M):
        raise ValueError('dump contains row data or environment ownership/security commands')
    if 'pg_catalog.setval(' in commands:
        raise ValueError('dump contains sequence state')
    return result


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    if args.output.exists():
        parser.error('output already exists; choose a new path')
    env = os.environ.copy()
    # Force the source session read-only, including any function-default checks.
    env['PGOPTIONS'] = env.get('PGOPTIONS', '') + ' -c default_transaction_read_only=on -c lock_timeout=5000'
    command = ['pg_dump', '--schema-only', '--schema=public', '--no-owner', '--no-privileges',
               '--no-comments', '--no-security-labels', '--no-tablespaces', '--no-publications',
               '--no-subscriptions', '--quote-all-identifiers', '--exclude-table=public.goose_db_version']
    result = subprocess.run(command, env=env, capture_output=True)
    if result.returncode:
        raise SystemExit('schema export failed; connection details withheld')
    sql = sanitize_dump(result.stdout.decode('utf-8'))
    args.output.parent.mkdir(parents=True, exist_ok=True)
    descriptor = os.open(args.output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(descriptor, 'w') as file:
        file.write('-- PostgreSQL public schema; structure only, no application rows or migration ledger.\n')
        file.write(sql)
    print('Schema-only export written; no credentials or business rows emitted.')


if __name__ == '__main__':
    main()
