"""Bounded source-project packaging; no build backend, installer or network."""
from contextlib import contextmanager
import hashlib
import importlib
import json
from pathlib import Path, PurePosixPath
import re
import sys
import tomllib

from .agent import AgentAdapter


def module_adapter(name):
    module = importlib.import_module(name)
    adapters = {id(value): value for value in vars(module).values() if isinstance(value, AgentAdapter)}
    if not adapters and hasattr(module, '__path__'):
        module = importlib.import_module(name + '.__main__')
        adapters = {id(value): value for value in vars(module).values() if isinstance(value, AgentAdapter)}
    if len(adapters) != 1 or 'execution.start' not in next(iter(adapters.values()))._handlers:
        raise ValueError('exactly one exported AgentAdapter required')
    return next(iter(adapters.values()))


def safe_tree(directory):
    files = []
    total = 0
    for path in sorted(directory.rglob('*')):
        if path.is_symlink():
            raise ValueError('project symlinks forbidden')
        if any(part in {'__pycache__', '.git', '.venv', 'venv', '.pytest_cache'} for part in path.relative_to(directory).parts):
            continue
        if path.is_dir():
            continue
        if not path.is_file() or path.name.startswith('.env') or path.suffix.lower() in {'.pem', '.key', '.p12', '.pfx'}:
            raise ValueError('unsafe project file')
        size = path.stat().st_size
        total += size
        if len(files) >= 1800 or size > (1 << 20) or total > (64 << 20):
            raise ValueError('project file limit')
        files.append(path)
    return files


def project_files(source, module):
    if not re.fullmatch(r'[A-Za-z_][A-Za-z0-9_]*(\.[A-Za-z_][A-Za-z0-9_]*)*', module):
        raise ValueError('invalid module')
    root = source.resolve(strict=True)
    config_path = root / 'pyproject.toml'
    if config_path.is_symlink() or config_path.stat().st_size > (1 << 20):
        raise ValueError('invalid project metadata')
    config = tomllib.loads(config_path.read_text())
    if not isinstance(config.get('project'), dict):
        raise ValueError('pyproject project table required')
    top = module.split('.')[0]
    if top in ('seven_agent', 'seven_workforce_runtime') or top in sys.stdlib_module_names:
        raise ValueError('reserved runtime or standard-library package')
    roots = [base for base in (root / 'src', root) if (base / top / '__init__.py').is_file()]
    if len(roots) != 1:
        raise ValueError('one src or flat regular package required')
    base = roots[0]
    package = base / top
    if base.is_symlink() or package.is_symlink():
        raise ValueError('package symlinks forbidden')
    files = safe_tree(package)
    selected = {str(path.relative_to(base)): path.read_bytes() for path in files if path.suffix == '.py'}
    data = config.get('tool', {}).get('setuptools', {}).get('package-data', {})
    if not isinstance(data, dict):
        raise ValueError('invalid package-data')
    packages = [path.parent for path in files if path.name == '__init__.py']
    for key, patterns in data.items():
        if not isinstance(patterns, list) or any(not isinstance(pattern, str) for pattern in patterns):
            raise ValueError('invalid package-data patterns')
        targets = packages if key == '*' else [base.joinpath(*key.split('.'))]
        if key != '*' and (not key.startswith(top + '.') and key != top):
            raise ValueError('package-data outside selected package')
        for target in targets:
            if target not in packages:
                raise ValueError('package-data package missing')
            for pattern in patterns:
                if not pattern or PurePosixPath(pattern).is_absolute() or '..' in PurePosixPath(pattern).parts or '\\' in pattern:
                    raise ValueError('unsafe package-data pattern')
                matches = [path for path in target.glob(pattern) if path in files]
                if not matches:
                    raise ValueError('declared resource missing')
                for path in matches:
                    selected[str(path.relative_to(base))] = path.read_bytes()
    for body in selected.values():
        if re.search(rb'-----BEGIN (?:[A-Z0-9 ]+ )?PRIVATE KEY-----', body):
            raise ValueError('private key material forbidden')
    return base, selected


@contextmanager
def loaded_project(source, name):
    base, files = project_files(source, name)
    prefix = name.split('.')[0]
    old_modules = {key: value for key, value in sys.modules.items() if key == prefix or key.startswith(prefix + '.')}
    old_path, old_bytecode = sys.path[:], sys.dont_write_bytecode
    for key in old_modules: del sys.modules[key]
    sys.path.insert(0, str(base)); sys.dont_write_bytecode = True
    importlib.invalidate_caches()
    try:
        yield module_adapter(name), files
    finally:
        sys.path[:] = old_path; sys.dont_write_bytecode = old_bytecode
        for key in list(sys.modules):
            if key == prefix or key.startswith(prefix + '.'): del sys.modules[key]
        sys.modules.update(old_modules)


def build_project(output, name, files, manifest):
    files = dict(files)
    # Bundle the exact stdlib-only SDK so isolated imports do not depend on cwd,
    # user site-packages, PYTHONPATH or an implicit package installation.
    sdk = Path(__file__).parent
    sdk_base = sdk.parent.parent
    files['seven_agent/__init__.py'] = (sdk.parent / '__init__.py').read_bytes()
    for package in (sdk, sdk_base / 'seven_workforce_runtime'):
        for path in safe_tree(package):
            if path.suffix == '.py':
                files[str(path.relative_to(sdk_base))] = path.read_bytes()
    files['runtime-entry.py'] = (
        'import sys\nfrom pathlib import Path\n'
        'sys.path.insert(0, str(Path(__file__).resolve().parent))\n'
        'from seven_agent.force._project import module_adapter\n'
        f'module_adapter({name!r}).serve()\n'
    ).encode()
    launcher = b'#!/bin/sh\nexec /usr/bin/python3 -I /opt/seven/adapter/runtime-entry.py\n'
    files['entrypoint'] = launcher
    if len(files) > 2048 or any(len(body) > (1 << 20) for body in files.values()) or sum(map(len, files.values())) > (64 << 20):
        raise ValueError('image index limits')
    index = json.dumps([{'path': name, 'sha256': hashlib.sha256(body).hexdigest()} for name, body in sorted(files.items())], sort_keys=True, separators=(',', ':')).encode()
    manifest = dict(manifest, binarySha256=hashlib.sha256(launcher).hexdigest(), sourceSha256=hashlib.sha256(index).hexdigest())
    files['source-index.json'] = index
    files['adapter-manifest.json'] = json.dumps({'adapter': manifest}, sort_keys=True).encode()
    # Validate all inputs before reserving a new directory. Never overwrite an
    # existing build, even an empty directory or a symlink.
    output.mkdir(parents=True, exist_ok=False)
    for name, body in sorted(files.items()):
        path = output / name
        path.parent.mkdir(parents=True, exist_ok=True)
        with path.open('xb') as stream: stream.write(body)
        path.chmod(0o555 if name == 'entrypoint' else 0o444)
