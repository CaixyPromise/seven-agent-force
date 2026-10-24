#!/usr/bin/env python3
"""Collect exact-version license evidence; never infer an unknown license as permissive."""
import argparse, base64, concurrent.futures, hashlib, io, json, re, subprocess, tarfile, urllib.parse, urllib.request, zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
CACHE = ROOT / '.local/license-evidence-cache'
OUT = ROOT / 'third_party'

def fetch(url):
    parsed = urllib.parse.urlparse(url)
    if parsed.scheme != 'https' or parsed.hostname not in {'registry.npmjs.org','raw.githubusercontent.com','proxy.golang.org'} or parsed.username or parsed.password:
        raise ValueError('HTTPS evidence required')
    CACHE.mkdir(parents=True, exist_ok=True)
    p = CACHE / hashlib.sha256(url.encode()).hexdigest()
    if not p.exists():
        req = urllib.request.Request(url, headers={'User-Agent': 'Seven-Agent-Force-license-review'})
        with urllib.request.urlopen(req, timeout=30) as response:
            value = response.read(64 * 1024 * 1024 + 1)
        if len(value) > 64 * 1024 * 1024:
            raise ValueError('evidence too large')
        p.write_bytes(value)
    return p.read_bytes()

def evidence(data, origin):
    digest = hashlib.sha256(data).hexdigest()
    p = OUT / 'licenses' / (digest + '.txt')
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_bytes(data)
    return {'file': 'licenses/' + p.name, 'sha256': digest, 'origin': origin}

def license_name(name):
    return name.lower().startswith(('license', 'licence', 'copying', 'notice', 'copyright'))

def npm_entry(x, installed, production):
    name, version = x['name'], x['version']
    url = 'https://registry.npmjs.org/' + urllib.parse.quote(name, safe='') + '/' + version
    p = installed.get((name, version))
    meta = json.loads(p.read_text()) if p else json.loads(fetch(url))
    declared = meta.get('license')
    if isinstance(declared, dict): declared = declared.get('type')
    if not isinstance(declared, str) or not declared:
        raise ValueError('missing npm license declaration: ' + name)
    files = []
    if p:
        files = [evidence(f.read_bytes(), url + '#package/' + f.name) for f in sorted(p.parent.iterdir()) if f.is_file() and license_name(f.name)]
    if not files:
        registry = json.loads(fetch(url))
        raw = fetch(registry['dist']['tarball'])
        integrity = registry['dist'].get('integrity', '')
        if integrity.startswith('sha512-'):
            assert base64.b64encode(hashlib.sha512(raw).digest()).decode() == integrity[7:]
        else:
            assert hashlib.sha1(raw).hexdigest() == registry['dist']['shasum']
        with tarfile.open(fileobj=io.BytesIO(raw)) as archive:
            for member in archive.getmembers():
                if member.isfile() and len(Path(member.name).parts) == 2 and license_name(Path(member.name).name):
                    files.append(evidence(archive.extractfile(member).read(), registry['dist']['tarball'] + '#' + member.name))
        if not files:
            repo = registry.get('repository', {})
            repo = repo if isinstance(repo, dict) else {'url': repo}
            match = re.search(r'github.com[:/]([^/]+/[^/#]+)', repo.get('url', ''))
            commit = registry.get('gitHead')
            if match and commit and re.fullmatch('[0-9a-f]{40}', commit):
                project = match.group(1).removesuffix('.git')
                for filename in ['LICENSE', 'LICENSE.md', 'LICENSE.txt', 'license', 'license.md', 'LICENSE-MIT', 'LICENCE', 'LICENSE-MIT.txt']:
                    rawurl = f'https://raw.githubusercontent.com/{project}/{commit}/{filename}'
                    try: data = fetch(rawurl)
                    except Exception: continue
                    files.append(evidence(data, rawurl)); break
        if not files:
            # Some older npm packages publish the complete license in README/code headers.
            with tarfile.open(fileobj=io.BytesIO(raw)) as archive:
                for member in archive.getmembers():
                    if not member.isfile() or member.size > 200000: continue
                    data = archive.extractfile(member).read()
                    if b'Permission is hereby granted' in data or b'Redistribution and use in source' in data:
                        if 'readme' in member.name.lower():
                            files.append(evidence(data, registry['dist']['tarball'] + '#' + member.name)); break
                        if data.lstrip().startswith(b'/*') and b'*/' in data:
                            header = data[:data.index(b'*/')+2]
                            if b'Redistribution and use' in header or b'Permission is hereby granted' in header:
                                files.append(evidence(header, registry['dist']['tarball'] + '#' + member.name + ':license-header')); break
        if not files:
            if (name, version) not in production:
                # Build/other-platform optional code is not included in the release.
                declaration = {key: registry.get(key) for key in ['name','version','license','author','repository']}
                files.append(evidence((json.dumps(declaration,indent=2)+'\n').encode(), url + '#license-declaration-only-not-redistributed'))
            else:
                raise ValueError('missing exact-version npm license text: ' + name + '@' + version)
    return dict(ecosystem='npm', name=name, version=version, license=declared,
                scope='web-production-dependency' if (name, version) in production else 'build-or-optional-dependency',
                source=url, notices=files)

def go_entry(x, gopath):
    name, version = x['name'], x['version']
    if name == 'github.com/CaixyPromise/seven-agent/force': return None
    if name == 'stdlib':
        goroot = Path(subprocess.check_output(['go','env','GOROOT'], text=True).strip())
        current = subprocess.check_output(['go','env','GOVERSION'], text=True).strip()
        if current != version: raise ValueError('stdlib evidence version mismatch')
        files = [evidence((goroot/'LICENSE').read_bytes(), f'https://go.googlesource.com/go/+/refs/tags/{version}/LICENSE')]
        return dict(ecosystem='go', name=name, version=version, license='BSD-3-Clause', scope='binary-runtime', source=f'https://go.dev/dl/{version}.src.tar.gz', notices=files)
    source = 'https://proxy.golang.org/' + ''.join('!' + c.lower() if c.isupper() else c for c in name) + '/@v/' + version + '.zip'
    files, labels, excluded = [], set(), []
    for lic in x.get('licenses', []):
        included = False
        for url in lic.get('urls', []):
            if '/testdata/' in url or url.endswith('LICENSE.docs'):
                excluded.append(url.replace('file://$GOPATH/pkg/mod/', 'go-module-cache:'))
                continue
            if url.startswith('file://$GOPATH/'):
                local = gopath / url[len('file://$GOPATH/'):]
                data = local.read_bytes()
                origin = source + '#' + str(local).split('/pkg/mod/', 1)[1]
            elif '.zip#' in url:
                zipurl, member = url.split('#', 1)
                with zipfile.ZipFile(io.BytesIO(fetch(zipurl))) as archive: data = archive.read(member)
                origin = url
            else:
                raise ValueError('unhandled Go evidence ' + name)
            files.append(evidence(data, origin)); included = True
        if included:
            label = lic.get('spdxExpression') or lic.get('value')
            labels.add(label)
    escaped = ''.join('!' + c.lower() if c.isupper() else c for c in name)
    module_root = gopath / 'pkg/mod' / (escaped + '@' + version)
    # Apache NOTICE/copyright files are not always recognized as license expressions.
    if module_root.is_dir():
        for notice in sorted(module_root.iterdir()):
            if notice.is_file() and license_name(notice.name) and notice.name != 'LICENSE.docs':
                item = evidence(notice.read_bytes(), source + '#' + escaped + '@' + version + '/' + notice.name)
                if item not in files: files.append(item)
    else:
        with zipfile.ZipFile(io.BytesIO(fetch(source))) as archive:
            for path in archive.namelist():
                relative = path.split('@' + version + '/',1)[-1]
                if '/' not in relative and license_name(relative) and relative != 'LICENSE.docs':
                    item = evidence(archive.read(path),source + '#' + path)
                    if item not in files: files.append(item)
    if name == 'github.com/golang/freetype':
        local = gopath / 'pkg/mod' / (name + '@' + version) / 'licenses/ftl.txt'
        files.append(evidence(local.read_bytes(), source + '#' + name + '@' + version + '/licenses/ftl.txt'))
        labels = {'FTL'}
    if not files or not labels or any(not l or l.startswith('sha256:') for l in labels):
        raise ValueError('unresolved Go license: ' + name)
    # Multiple detected notices are retained conjunctively, never silently changed to OR.
    entry = dict(ecosystem='go', name=name, version=version, license=' AND '.join(sorted(labels)),
                scope='binary-or-module-graph', source=source, notices=files, excluded_evidence=excluded)
    if 'MPL-2.0' in labels:
        entry['source_archive'] = dict(url=source, sha256=hashlib.sha256(fetch(source)).hexdigest(),
                                       file='sources/' + name.replace('/','_') + '-' + version + '.zip')
    return entry

INPUT_PATHS = ['seven-agent-force-server/go.mod','seven-agent-force-web/package.json','seven-agent-force-web/pnpm-lock.yaml']

def input_hashes(root):
    return {name:hashlib.sha256((root/name).read_bytes()).hexdigest() for name in INPUT_PATHS}

def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('sbom', type=Path)
    parser.add_argument('production_tree', type=Path)
    args = parser.parse_args()
    installed = {}
    for p in (ROOT / 'seven-agent-force-web/node_modules/.pnpm').glob('*/node_modules/**/package.json'):
        try:
            d = json.loads(p.read_text()); installed[(d.get('name'), d.get('version'))] = p
        except (ValueError, OSError): pass
    production = set()
    def walk(node):
        if isinstance(node, dict):
            for field in ['dependencies', 'optionalDependencies']:
                for name, child in node.get(field, {}).items():
                    production.add((name, child.get('version'))); walk(child)
        elif isinstance(node, list):
            for child in node: walk(child)
    walk(json.loads(args.production_tree.read_text()))
    unique = {(x['type'],x['name'],x['version']): x for x in json.loads(args.sbom.read_text())['artifacts']}
    gopath = Path(subprocess.check_output(['go','env','GOPATH'], text=True).strip())
    def process(x):
        if x['type'] == 'npm':
            if x['name'] == '@ant-design/use-emotion-css' and x['name'] not in (ROOT/'seven-agent-force-web/pnpm-lock.yaml').read_text(): return None
            return npm_entry(x, installed, production)
        if x['type'] == 'go-module': return go_entry(x, gopath)
        if x['type'] == 'python' and x['name'] in ['seven-workforce-runtime-sdk', 'seven-agent-force-runtime-sdk']: return None
        raise ValueError('unhandled package type')
    entries, errors = [], []
    with concurrent.futures.ThreadPoolExecutor(max_workers=6) as pool:
        futures = {pool.submit(process,x): key for key,x in unique.items()}
        for f in concurrent.futures.as_completed(futures):
            try:
                entry=f.result()
                if entry: entries.append(entry)
            except Exception as e: errors.append({'package': futures[f], 'error': str(e)})
    entries.sort(key=lambda x:(x['ecosystem'],x['name'],x['version']))
    OUT.mkdir(exist_ok=True)
    (OUT/'licenses.json').write_text(json.dumps({'schema':1,'inputs':input_hashes(ROOT),'entries':entries,'unresolved':errors},indent=2)+'\n')
    print(f'License entries {len(entries)}, unresolved {len(errors)}')
    for error in errors: print(error['package'], error['error'])
    return bool(errors)

if __name__ == '__main__': raise SystemExit(main())
