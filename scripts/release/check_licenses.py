#!/usr/bin/env python3
"""Fail closed on unreviewed package versions, licenses or changed notice text."""
import argparse, hashlib, json, re, shutil, tomllib
from collect_licenses import fetch, input_hashes
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
ALLOWED = {'MIT','Apache-2.0','BSD-2-Clause','BSD-3-Clause','ISC','0BSD','BSL-1.0',
           'Unlicense','MPL-2.0','Python-2.0','CC-BY-4.0','BlueOak-1.0.0','FTL','MIT-0'}


def validate_inventory(directory):
    data = json.loads((directory/'licenses.json').read_text())
    if data.get('unresolved'):
        raise ValueError('Unresolved license entries remain')
    entries = {}
    for entry in data['entries']:
        labels = set(re.split(r'\s+(?:AND|OR)\s+', entry['license']))
        if not labels or not labels <= ALLOWED:
            raise ValueError('Unreviewed license expression: ' + entry['name'])
        key = entry['name'], entry['version']
        if key in entries:
            raise ValueError('Duplicate license entry')
        if not entry.get('notices'):
            raise ValueError('No license evidence: '+entry['name'])
        for notice in entry['notices']:
            p = Path(notice['file'])
            if p.is_absolute() or '..' in p.parts or p.parts[0] != 'licenses':
                raise ValueError('Invalid license evidence path')
            if hashlib.sha256((directory/p).read_bytes()).hexdigest() != notice['sha256']:
                raise ValueError('Changed license evidence: '+entry['name'])
            if not notice['origin'].startswith('https://'):
                raise ValueError('Nonportable license evidence origin')
            if 'license-declaration-only' in notice['origin'] and entry['scope'] != 'build-or-optional-dependency':
                raise ValueError('Distributed package lacks full notice: '+entry['name'])
        if 'MPL-2.0' in labels and entry['ecosystem']=='go' and not entry.get('source_archive'):
            raise ValueError('MPL binary dependency lacks corresponding source archive')
        entries[key] = entry
    return entries


def validate_inputs(data, root):
    if data.get('inputs') != input_hashes(root):
        raise ValueError('Dependency inputs changed; refresh scope/version license review (including production promotions)')

def check_locked_dependencies(entries):
    lock = (ROOT/'seven-agent-force-web/pnpm-lock.yaml').read_text().split('packages:\n',1)[1].split('\nsnapshots:',1)[0]
    required = []
    for line in lock.splitlines():
        match = re.fullmatch(r"  (?:'([^']+)'|([^ :]+)):\s*", line)
        if match:
            name, version = (match[1] or match[2]).rsplit('@',1)
            required.append((name,version))
    for line in (ROOT/'seven-agent-force-server/go.mod').read_text().splitlines():
        match = re.fullmatch(r'\s+([^\s]+)\s+(v[^\s]+)(?:\s+// indirect)?',line)
        if match: required.append((match[1],match[2]))
    missing = [key for key in required if key not in entries]
    if missing: raise ValueError('Locked dependencies absent from license review: '+repr(missing))
    return len(required)


def annotate_sbom(data, entries, sdk_version):
    count = 0
    for package in data['packages']:
        key = package.get('name'), package.get('versionInfo')
        if key in entries:
            expression = entries[key]['license']
            package['licenseDeclared'] = expression
            package['licenseComments'] = 'Reviewed exact-version evidence: third_party/licenses.json. Detected test/document-only notices are scoped in that inventory.'
        elif key[0] == 'github.com/CaixyPromise/seven-agent/force' or (key[0]=='seven-agent-force-runtime-sdk' and key[1]==sdk_version) or key==('seven-agent-force-web','0.1.0') or package.get('primaryPackagePurpose') == 'FILE' and re.fullmatch(r'seven-agent-force_[0-9A-Za-z._-]+_(linux|darwin)_(amd64|arm64)',key[0]):
            package['licenseDeclared'] = 'Apache-2.0'
        else:
            raise ValueError('SBOM package not reviewed: '+repr(key))
        count += 1
    # Syft's file cataloger does not always identify a source-only pyproject SDK.
    # Keep the explicitly shipped first-party SDK in the SBOM rather than silently omitting it.
    if not any(p.get('name')=='seven-agent-force-runtime-sdk' for p in data['packages']):
        identifier='SPDXRef-ForceRuntimeSDK-'+sdk_version.replace('.','-')
        data['packages'].append(dict(name='seven-agent-force-runtime-sdk',SPDXID=identifier,
            versionInfo=sdk_version,downloadLocation='NOASSERTION',filesAnalyzed=False,
            licenseDeclared='Apache-2.0',licenseConcluded='NOASSERTION',
            copyrightText='Copyright 2026 CaixyPromise',primaryPackagePurpose='LIBRARY',
            sourceInfo='Source included at deploy/workforce/runtime-sdk; identity from pyproject.toml',
            externalRefs=[dict(referenceCategory='PACKAGE-MANAGER',referenceType='purl',
                               referenceLocator='pkg:pypi/seven-agent-force-runtime-sdk@'+sdk_version)]))
        for root in data.get('documentDescribes',[]):
            data.setdefault('relationships',[]).append(dict(spdxElementId=root,relationshipType='CONTAINS',relatedSpdxElement=identifier))
        count += 1
    return count


def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('--sbom',type=Path)
    parser.add_argument('--stage',type=Path)
    args=parser.parse_args()
    entries=validate_inventory(ROOT/'third_party')
    validate_inputs(json.loads((ROOT/'third_party/licenses.json').read_text()),ROOT)
    locked=check_locked_dependencies(entries)
    if args.sbom:
        project=tomllib.loads((ROOT/'deploy/workforce/runtime-sdk/pyproject.toml').read_text())['project']
        if project['name']!='seven-agent-force-runtime-sdk' or project['license']!={'text':'Apache-2.0'}:
            raise ValueError('First-party SDK identity/license changed; review required')
        sdk=project['version']
        data=json.loads(args.sbom.read_text())
        count=annotate_sbom(data,entries,sdk)
        args.sbom.write_text(json.dumps(data,indent=2)+'\n')
        print(f'SBOM: {count} packages annotated; zero unreviewed packages')
    if args.stage:
        shutil.copytree(ROOT/'third_party',args.stage/'third_party',dirs_exist_ok=True)
        shutil.copy2(ROOT/'THIRD_PARTY_NOTICES.md',args.stage/'THIRD_PARTY_NOTICES.md')
        for entry in entries.values():
            if not entry.get('source_archive'): continue
            source=entry['source_archive']
            data=fetch(source['url'])
            if hashlib.sha256(data).hexdigest()!=source['sha256']: raise ValueError('Corresponding source checksum mismatch')
            target=args.stage/'third_party'/source['file']
            if Path(source['file']).is_absolute() or '..' in Path(source['file']).parts: raise ValueError('Invalid source archive path')
            target.parent.mkdir(parents=True,exist_ok=True);target.write_bytes(data)
    print(f'License gate PASS: {len(entries)} exact-version entries; {locked} locked dependencies checked')

if __name__=='__main__': main()
