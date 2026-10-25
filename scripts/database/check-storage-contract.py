#!/usr/bin/env python3
"""Read-only storage-contract preflight. Emits counts and column/constraint names,
never row contents or connection settings. Supply libpq environment/service config.
"""
import argparse
import json
import os
from pathlib import Path
import re
import subprocess

ROOT = Path(__file__).resolve().parents[2]


def query(sql):
    env = dict(os.environ)
    env['PGOPTIONS'] = env.get('PGOPTIONS', '') + ' -c default_transaction_read_only=on -c statement_timeout=30000 -c lock_timeout=5000'
    result = subprocess.run(['psql', '-XAt', '-v', 'ON_ERROR_STOP=1', '-c', sql], env=env, capture_output=True, text=True)
    if result.returncode:
        raise SystemExit('Read-only preflight failed; connection and row details withheld.')
    return result.stdout.strip()


def check():
    sql = (ROOT / 'seven-agent-force-server/internal/app/workforce/schemacontract/columns.sql').read_text()
    targets = list(dict.fromkeys(re.findall(r"\('([a-z_0-9]+)','([a-z_0-9]+)','([^']+)',(\d+),(true|false)\)", sql)))
    if len(targets) != 280:
        raise SystemExit('Storage policy format changed; update the reviewed preflight parser.')
    issues = []
    for table, column, target, limit, required in targets:
        conditions = []
        if int(limit):
            conditions.append(('LENGTH', f'char_length("{column}"::text)>{int(limit)}'))
        if target == 'smallint':
            conditions.append(('INTEGER_RANGE', f'"{column}" NOT BETWEEN -32768 AND 32767'))
        if required == 'true':
            conditions.append(('REQUIRED', f'"{column}" IS NULL'))
        if not conditions:
            continue
        counts = query('SELECT ' + ','.join(f'count(*) FILTER (WHERE {predicate})' for _, predicate in conditions) + f' FROM public."{table}";').split('|')
        for (reason, _), value in zip(conditions, counts):
            if int(value):
                issues.append(dict(column=table+'.'+column, reason=reason, count=int(value)))
    guards = (ROOT / 'seven-agent-force-server/internal/app/workforce/schemacontract/guards.sql').read_text()
    checked = 0
    for table, name, expression in re.findall(r'ALTER TABLE public\.([a-z_0-9]+) ADD CONSTRAINT ([a-z_0-9]+) CHECK \((.*?)\) NOT VALID;', guards):
        count = int(query(f'SELECT count(*) FROM public."{table}" WHERE NOT ({expression});'))
        checked += 1
        if count:
            issues.append(dict(constraint=table+'.'+name, reason='CHECK', count=count))
    return dict(column_targets=len(targets), guard_targets=checked, ready=not issues, issues=issues)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--output', type=Path, required=True)
    args = parser.parse_args()
    if args.output.exists():
        parser.error('output already exists')
    result = check()
    descriptor = os.open(args.output, os.O_WRONLY | os.O_CREAT | os.O_EXCL, 0o600)
    with os.fdopen(descriptor, 'w') as file:
        json.dump(result, file, indent=2)
        file.write('\n')
    print('Read-only preflight complete: '+('READY' if result['ready'] else 'BLOCKED')+'; no row values exported.')
    return 0 if result['ready'] else 2


if __name__ == '__main__':
    raise SystemExit(main())
