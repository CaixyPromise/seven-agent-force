#!/usr/bin/env python3
"""Validate documentation links, anchors and source-inventory coverage; no live calls."""
import hashlib
import json
import re
from collections import Counter
from pathlib import Path
from urllib.parse import unquote
BASE=Path(__file__).resolve().parents[1]
ROOT=BASE.parents[2]
x=json.loads((BASE/'contracts.json').read_text())
errors=[]
keys=[(r['Method'],r['Path']) for r in x['routes']]
assert len(keys)==len(set(keys))==231
counts=Counter('open' if p.startswith('/api/open/') else 'provider' if p.startswith('/api/internal/workforce/v1/model-invocations/') else 'worker' if p.startswith('/api/internal/') else 'operator' for _,p in keys)
assert counts=={'open':7,'provider':4,'worker':86,'operator':134},counts
names={t['Name'] for t in x['types']}
for t in x['types']:
    for f in t.get('Fields') or []:
        if f['Reference'] and f['Reference'] not in names:errors.append('unresolved type '+f['Reference'])
for group,count in counts.items():
    txt=(BASE/f'http-{group}.md').read_text()
    if len(re.findall(r'^## (GET|POST|PUT|DELETE|PATCH|HEAD|OPTIONS) `',txt,re.M))!=count:errors.append('route coverage '+group)
    for method,path in keys:
        g='open' if path.startswith('/api/open/') else 'provider' if path.startswith('/api/internal/workforce/v1/model-invocations/') else 'worker' if path.startswith('/api/internal/') else 'operator'
        if group==g and f'## {method} `{path}`' not in txt:errors.append('undocumented '+method+' '+path)
# Local file existence; generated explicit anchors and source line ranges.
checks=0
for path in sorted(BASE.rglob('*.md')):
    txt=path.read_text()
    for target in re.findall(r'\]\(([^\s)]+)\)',txt):
        if '://' in target:continue
        name,_,fragment=target.partition('#')
        dest=(path.parent/unquote(name)).resolve() if name else path
        if not dest.exists():errors.append(f'{path.name}: missing {target}');continue
        checks+=1
        if fragment.startswith('t-') or fragment.startswith('route-'):
            if f'id="{fragment}"' not in dest.read_text():errors.append(f'{path.name}: missing anchor {target}')
        elif fragment.startswith('L') and fragment[1:].isdigit():
            if int(fragment[1:])>len(dest.read_text().splitlines()):errors.append(f'{path.name}: invalid line {target}')
# Every allowed Adapter method is represented by a row.
codec=(ROOT/'seven-agent-force-server/internal/worker/agentprotocol/codec.go').read_text()
allowed=codec[codec.index('func allowed('):codec.index('// strictValue')]
methods=set(re.findall(r'"([\w.]+)"',allowed))-{'up','down'}
adapter=(BASE/'agent-adapter.md').read_text()
assert len(methods)==25
for m in methods:
    if f'| {m} |' not in adapter:errors.append('undocumented Adapter '+m)
if errors:
    print('\n'.join(errors));raise SystemExit(1)
print(f'PASS: {len(keys)} routes, {len(names)} type graphs, {len(methods)} Adapter methods, {checks} local links/anchors checked')
