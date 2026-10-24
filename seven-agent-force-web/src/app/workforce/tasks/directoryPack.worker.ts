import { BlobReader, BlobWriter, ZipWriter } from '@zip.js/zip.js';
import { createSHA256 } from 'hash-wasm';

type Request = { files: File[] };
const ignored = new Set(['node_modules','target','__pycache__','.pytest_cache','.mypy_cache','.ruff_cache','.venv','venv','dist','build','.next','.nuxt','coverage','.gradle','bin','obj','tmp','temp','.git']);

self.onmessage = async (event: MessageEvent<Request>) => {
  try {
    const selected = event.data.files
      .map((file) => ({ file, path: file.webkitRelativePath || file.name }))
      .filter(({ path }) => !path.split('/').some((part) => ignored.has(part)))
      .sort((a, b) => a.path.localeCompare(b.path, 'en'));
    const output = new BlobWriter('application/zip');
    const writer = new ZipWriter(output, { bufferedWrite: true });
    for (const { file, path } of selected) {
      await writer.add(path, new BlobReader(file), { lastModDate: new Date('1980-01-01T00:00:00Z'), level: 6 });
    }
    const archive = await writer.close();
    const sha = await createSHA256();
    for (let offset = 0; offset < archive.size; offset += 4 * 1024 * 1024) sha.update(new Uint8Array(await archive.slice(offset, offset + 4 * 1024 * 1024).arrayBuffer()));
    self.postMessage({ archive, sha256: sha.digest(), fileCount: selected.length, excludedCount: event.data.files.length - selected.length });
  } catch {
    self.postMessage({ error: 'DIRECTORY_PACK_FAILED' });
  }
};
