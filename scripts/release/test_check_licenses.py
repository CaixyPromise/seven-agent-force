import copy
import hashlib
import importlib.util
import json
from pathlib import Path
import sys
import tempfile
import unittest

sys.path.insert(0,str(Path(__file__).parent))
from check_licenses import annotate_sbom, validate_inventory, validate_inputs
from collect_licenses import input_hashes, INPUT_PATHS

class LicenseGateTests(unittest.TestCase):
    def setUp(self):
        self.tmp=tempfile.TemporaryDirectory();self.addCleanup(self.tmp.cleanup)
        self.root=Path(self.tmp.name);(self.root/'licenses').mkdir()
        data=b'Synthetic license test evidence'
        (self.root/'licenses/test.txt').write_bytes(data)
        self.entry=dict(name='example',version='1.0.0',ecosystem='npm',license='MIT',scope='web-production-dependency',
                        notices=[dict(file='licenses/test.txt',sha256=hashlib.sha256(data).hexdigest(),origin='https://example.invalid/LICENSE')])
    def write(self,entry=None,unresolved=None):
        (self.root/'licenses.json').write_text(json.dumps(dict(entries=[entry or self.entry],unresolved=unresolved or [])))
    def test_known_version_and_notice(self):
        self.write();entries=validate_inventory(self.root)
        data=dict(packages=[dict(name='example',versionInfo='1.0.0')]);annotate_sbom(data,entries,'0.1.0')
        self.assertEqual(data['packages'][0]['licenseDeclared'],'MIT')
    def test_unknown_package_or_version_rejected(self):
        self.write();entries=validate_inventory(self.root)
        for name,version in [('example','2.0.0'),('new-package','1.0.0'),('seven-workforce-runtime-sdk','0.1.0')]:
            with self.assertRaises(ValueError): annotate_sbom(dict(packages=[dict(name=name,versionInfo=version)]),entries,'0.1.0')
    def test_shipped_sdk_is_present_exactly_once(self):
        self.write();entries=validate_inventory(self.root)
        data=dict(packages=[dict(name='example',versionInfo='1.0.0')])
        annotate_sbom(data,entries,'0.1.0');annotate_sbom(data,entries,'0.1.0')
        sdk=[p for p in data['packages'] if p['name']=='seven-agent-force-runtime-sdk']
        self.assertEqual(len(sdk),1);self.assertEqual(sdk[0]['versionInfo'],'0.1.0')
        self.assertEqual(sdk[0]['licenseDeclared'],'Apache-2.0')

    def test_changed_notice_rejected(self):
        self.write();(self.root/'licenses/test.txt').write_bytes(b'replaced')
        with self.assertRaises(ValueError):validate_inventory(self.root)
    def test_unreviewed_license_rejected(self):
        e=copy.deepcopy(self.entry);e['license']='GPL-2.0-only';self.write(e)
        with self.assertRaises(ValueError):validate_inventory(self.root)
    def test_runtime_declaration_only_rejected(self):
        e=copy.deepcopy(self.entry);e['notices'][0]['origin']='https://example.invalid/#license-declaration-only';self.write(e)
        with self.assertRaises(ValueError):validate_inventory(self.root)
    def test_mpl_requires_source(self):
        e=copy.deepcopy(self.entry);e.update(license='MPL-2.0',ecosystem='go');self.write(e)
        with self.assertRaises(ValueError):validate_inventory(self.root)
    def test_dependency_promotion_requires_new_review(self):
        for name in INPUT_PATHS:
            p=self.root/name;p.parent.mkdir(parents=True,exist_ok=True);p.write_text('original')
        data=dict(inputs=input_hashes(self.root));validate_inputs(data,self.root)
        (self.root/'seven-agent-force-web/package.json').write_text('promoted dependency to production')
        with self.assertRaises(ValueError):validate_inputs(data,self.root)

    def test_unresolved_entry_rejected(self):
        self.write(unresolved=['missing'])
        with self.assertRaises(ValueError):validate_inventory(self.root)

if __name__=='__main__': unittest.main()
