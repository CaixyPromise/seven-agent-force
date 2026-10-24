#!/usr/bin/env python3
"""Filename-only history checks, including snapshot trees invisible in patch scans."""
import argparse, functools, json, re, subprocess
from pathlib import Path

FORBIDDEN = re.compile(r'(^|/)(admin-pwd\.txt|application\.yaml)$|(^|/)\.env($|\.)|^\.local/|\.(pem|key|dump)$|auth\.png$')

@functools.lru_cache(maxsize=None)
def tree_paths(root, tree):
    # Scan names from each distinct commit tree, not deduplicated blob paths:
    # identical blobs can occur under a public and a forbidden filename.
    raw=subprocess.check_output(['git','-C',str(root),'ls-tree','-rz','--name-only',tree])
    return [name.decode('utf-8',errors='replace') for name in raw.split(b'\0') if name]

def paths(root, *refs):
    trees=subprocess.check_output(['git','-C',str(root),'log','--format=%T',*refs],text=True).splitlines()
    if '--all' in refs:
        refs_output=subprocess.check_output(['git','-C',str(root),'for-each-ref','--format=%(refname)'],text=True)
        for ref in refs_output.splitlines():
            # Codex capture refs can point directly at trees, without a commit.
            trees.append(subprocess.check_output(['git','-C',str(root),'rev-parse',ref+'^{tree}'],text=True).strip())
    return sorted({name for tree in set(trees) for name in tree_paths(str(root),tree)
                   if FORBIDDEN.search(name) and not name.endswith('/.env.example') and name!='.env.example'})

def main():
    p=argparse.ArgumentParser();p.add_argument('--root',type=Path,required=True);p.add_argument('--report',type=Path,required=True)
    p.add_argument('--candidate-only',action='store_true');p.add_argument('--scope',choices=['all','candidate'],default='all');args=p.parse_args()
    candidate=paths(args.root,'HEAD');all_paths=paths(args.root,'--all')
    data=dict(candidate=candidate,all_refs=all_paths,extra_ref_only=sorted(set(all_paths)-set(candidate)))
    args.report.write_text(json.dumps(data,indent=2)+'\n')
    print(f'Historical forbidden filenames: candidate {len(candidate)}; all local refs {len(all_paths)}')
    if all_paths and not candidate:print('Private extra refs must never be mirrored or included in public export; original refs preserved.')
    return bool(candidate or (all_paths and not (args.candidate_only or args.scope=='candidate')))

if __name__=='__main__':raise SystemExit(main())
