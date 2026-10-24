#!/usr/bin/env python3
"""Adjudicate scanner findings using exact, reviewed source spans. Never emit secrets."""
import argparse
import hashlib
import json
from pathlib import Path
import subprocess


def identity(finding, scope, root, source_root=None):
    path = finding['File']
    if scope == 'source':
        path = Path(path).relative_to(source_root).as_posix() if Path(path).is_absolute() else path.removeprefix('./')
    if Path(path).is_absolute() or '..' in Path(path).parts:
        raise ValueError('invalid scanner path')
    commit = finding.get('Commit', '') if scope == 'history' else 'HEAD'
    # A missing blob is a check failure, never an accepted finding.
    blob = (source_root/path).read_bytes() if scope=='source' and source_root else subprocess.check_output(['git','-C',str(root),'show',f'{commit}:{path}'])
    lines = blob.splitlines(keepends=True)
    start, end = finding['StartLine'], finding['EndLine']
    if not (1 <= start <= end <= len(lines)):
        raise ValueError('invalid scanner line range')
    return dict(scope=scope, path=path, rule=finding['RuleID'],
                start=start, end=end, commit=commit if scope == 'history' else '',
                sha256=hashlib.sha256(b''.join(lines[start-1:end])).hexdigest())


def adjudicate(findings, reviewed, scope, root, source_root=None):
    accepted, unresolved = [], []
    for finding in findings:
        item = identity(finding, scope, root, source_root)
        match = next((r for r in reviewed if all(r.get(k) == v for k, v in item.items())
                      and r.get('decision') == 'false_positive' and r.get('reason')), None)
        if not match and scope=='history':
            parents=subprocess.check_output(['git','-C',str(root),'rev-list','--parents','-n','1',item['commit']],text=True).split()
            if len(parents)==1:
                match=next((r for r in reviewed if r.get('scope')=='source' and r.get('decision')=='false_positive' and r.get('reason') and all(r.get(k)==item[k] for k in ['path','rule','start','end','sha256'])),None)
        (accepted if match else unresolved).append(item)
    return {'scope': scope, 'accepted': accepted, 'unresolved': unresolved}


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('scope', choices=['source', 'history'])
    parser.add_argument('report', type=Path)
    parser.add_argument('--root', type=Path, required=True)
    parser.add_argument('--source-root', type=Path)
    args = parser.parse_args()
    findings = json.loads(args.report.read_text())
    if not isinstance(findings, list):
        raise ValueError('invalid scanner report')
    reviewed = json.loads((args.root / 'scripts/release/reviewed-secret-findings.json').read_text())['findings']
    result = adjudicate(findings, reviewed, args.scope, args.root, args.source_root)
    args.report.with_suffix('.review.json').write_text(json.dumps(result, indent=2) + '\n')
    print(f"{args.scope}: {len(findings)} raw; {len(result['accepted'])} exact reviewed false positives; {len(result['unresolved'])} unresolved")
    return bool(result['unresolved'])

if __name__ == '__main__':
    raise SystemExit(main())
