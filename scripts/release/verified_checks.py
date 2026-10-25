#!/usr/bin/env python3
"""Verify GitHub CI evidence before reusing checks; never accepts caller verdicts."""
import argparse
import json
import subprocess
from pathlib import Path

REPO = 'CaixyPromise/seven-agent-force'
REQUIRED = {'server', 'web', 'secrets'}
# Only release assembly and explanatory docs may differ from the checked commit.
RELEASE_ONLY = {'scripts/release/build.sh', 'scripts/release/verified_checks.py',
                'scripts/release/test_verified_checks.py', 'docs/deployment.md',
                'docs/workforce/worker-container.md'}


def validate(run, jobs, changed):
    if run.get('repository', {}).get('full_name') != REPO:
        raise ValueError('CI repository mismatch')
    if run.get('event') != 'push' or run.get('path') != '.github/workflows/ci.yml':
        raise ValueError('unexpected CI workflow or event')
    if run.get('status') != 'completed' or run.get('conclusion') != 'success':
        raise ValueError('CI is not completed successfully')
    names = [j['name'] for j in jobs]
    for name in REQUIRED:
        matches = [j for j in jobs if j['name'] == name]
        if names.count(name) != 1 or matches[0].get('conclusion') != 'success' or matches[0].get('status') != 'completed':
            raise ValueError('required CI job is missing or unsuccessful: ' + name)
    unexpected = set(changed) - RELEASE_ONLY
    if unexpected:
        raise ValueError('checked inputs changed: ' + ', '.join(sorted(unexpected)))


def command(root, *args):
    return subprocess.check_output(args, cwd=root, text=True).strip()


def verify(root, run_id, output):
    if not run_id.isdecimal():
        raise ValueError('CI run ID must be decimal')
    if command(root, 'git', 'status', '--porcelain', '--untracked-files=normal'):
        raise ValueError('release requires a clean source checkout')
    run = json.loads(command(root, 'gh', 'api', f'repos/{REPO}/actions/runs/{run_id}'))
    attempt = run['run_attempt']
    response = json.loads(command(root, 'gh', 'api', f'repos/{REPO}/actions/runs/{run_id}/attempts/{attempt}/jobs?per_page=100'))
    if response['total_count'] != len(response['jobs']):
        raise ValueError('incomplete CI job response')
    checked = run['head_sha']
    head = command(root, 'git', 'rev-parse', 'HEAD')
    subprocess.run(['git', 'merge-base', '--is-ancestor', checked, head], cwd=root, check=True)
    changed = command(root, 'git', 'diff', '--name-only', checked, head).splitlines()
    validate(run, response['jobs'], changed)
    # Local Web overrides would produce different assets despite identical Git input.
    web = root / 'seven-agent-force-web'
    overrides = [p.name for p in web.glob('.env*') if p.name != '.env.example']
    import os
    if overrides or any(k.startswith('VITE_') for k in os.environ):
        raise ValueError('private Web build overrides are not allowed')
    evidence = {'schema': 'seven.force.release-checks/v1', 'sourceCommit': head,
                'checkedCommit': checked, 'ciRun': run['html_url'], 'ciAttempt': attempt,
                'reusedJobs': sorted(REQUIRED), 'releaseOnlyChanges': changed,
                'scope': 'unchanged application inputs; release assembly validated separately'}
    output.write_text(json.dumps(evidence, indent=2) + '\n')
    print('Verified CI evidence for unchanged application inputs: ' + run['html_url'])


if __name__ == '__main__':
    p = argparse.ArgumentParser()
    p.add_argument('--run', required=True)
    p.add_argument('--output', required=True, type=Path)
    args = p.parse_args()
    verify(Path(__file__).resolve().parents[2], args.run, args.output)
