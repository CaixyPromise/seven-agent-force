import assert from 'node:assert/strict';
import { readFile } from 'node:fs/promises';
import test from 'node:test';
import ts from 'typescript';

async function readSource(path) {
  return readFile(new URL(path, import.meta.url), 'utf8');
}

const uploadContractSource = await readSource('../src/api/uploadContract.ts');
const { outputText, diagnostics = [] } = ts.transpileModule(uploadContractSource, {
  compilerOptions: {
    module: ts.ModuleKind.ESNext,
    target: ts.ScriptTarget.ES2022,
  },
  reportDiagnostics: true,
});
assert.deepEqual(diagnostics, []);

const uploadContract = await import(
  `data:text/javascript;base64,${Buffer.from(outputText).toString('base64')}`
);

test('uses Go file-check hits to build the current faster-upload request', () => {
  assert.equal(uploadContract.isExistingFile({ exists: false }), false);
  assert.equal(uploadContract.isExistingFile({ exists: true }), false);
  assert.equal(uploadContract.isExistingFile({ exists: true, fileId: '9007199254740993' }), true);

  assert.deepEqual(uploadContract.buildFasterUploadInput({
    bizType: 1,
    bizId: '9007199254740993',
    fileName: 'report.pdf',
    contentType: 'application/pdf',
    sha256: 'abc123',
    fileSize: 4096,
  }), {
    fileName: 'report.pdf',
    contentType: 'application/pdf',
    sha256: 'abc123',
    fileSize: 4096,
  });
});

test('upload receipts do not expose a visit URL or reference authority', () => {
  assert.equal(uploadContract.resolveUploadUrl, undefined);
  assert.deepEqual(uploadContract.buildFasterUploadInput({
    fileName: 'report.pdf',
    contentType: 'application/pdf',
    sha256: 'abc123',
    fileSize: 4096,
    visitUrl: '/visit/1',
    referenceId: '2',
  }), {
    fileName: 'report.pdf',
    contentType: 'application/pdf',
    sha256: 'abc123',
    fileSize: 4096,
  });
});

test('upload success requires only a decimal file identifier', async () => {
  assert.equal(uploadContract.isAcceptedUploadResult(), false);
  assert.equal(uploadContract.isAcceptedUploadResult({ fileId: 'not-an-id' }), false);
  assert.equal(uploadContract.isAcceptedUploadResult({ referenceId: '2' }), false);
  assert.equal(uploadContract.isAcceptedUploadResult({ fileId: '1' }), true);
  assert.equal(uploadContract.isAcceptedUploadResult({ fileId: '9007199254740993' }), true);

  const modal = await readSource('../src/app/system/files/components/UploadModal.tsx');
  assert.match(modal, /res\.code === 0 && isAcceptedUploadResult\(res\.data\)/);
  assert.match(modal, /res\.code !== 0 \|\| !isAcceptedUploadResult\(res\.data\)/);
});

test('chunk and ordinary uploads use the upload-only contract', async () => {
  const [component, modal, controller, fileController] = await Promise.all([
    readSource('../src/components/ChunkUpload/index.tsx'),
    readSource('../src/app/system/files/components/UploadModal.tsx'),
    readSource('../src/api/chunkUploadController.ts'),
    readSource('../src/api/fileController.ts'),
  ]);

  assert.doesNotMatch(component, /data\?\.token|获取上传token|challenge/);
  assert.doesNotMatch(modal, /data\?\.token|if \(!token\)|challenge/);
  assert.match(component, /initChunkUpload\(\{/);
  assert.match(modal, /initChunkUpload\(\{/);
  assert.match(controller, /request<API\.ResultChunkPartResponse>/);
  assert.match(controller, /request<API\.ResultUploadResult>/);
  assert.doesNotMatch(controller, /bizId|bizType|referenceId/);
  assert.doesNotMatch(fileController, /uploadRequest\?\.biz|referenceId/);
});

test('apply-pending consumes the Result envelope instead of treating it as a number', async () => {
  const [controller, component] = await Promise.all([
    readSource('../src/api/configController.ts'),
    readSource('../src/app/system/config/components/PendingConfigList.tsx'),
  ]);

  assert.match(controller, /request<API\.Result<number>>\(`\/api\/config\/apply-pending`/);
  assert.match(component, /const count = response\.data \?\? 0/);
});

test('selector contracts are not replaced with privileged admin list/detail routes', async () => {
  const controller = await readSource('../src/api/userController.ts');
  const selectorSection = controller.slice(controller.indexOf('/** 获取用户选项列表'));

  assert.match(selectorSection, /`\/api\/user\/options`/);
  assert.match(selectorSection, /`\/api\/user\/search`/);
  assert.match(selectorSection, /`\/api\/user\/simple\/\$\{params\.id\}`/);
  assert.doesNotMatch(selectorSection, /`\/api\/user\/list\/page`/);
  assert.doesNotMatch(selectorSection, /`\/api\/user\/get\/\$\{params\.id\}`/);
});

test('keeps identifiers as Int64 strings while numeric metrics remain numbers', async () => {
  const typings = await readSource('../src/api/typings.d.ts');

  assert.match(typings, /type Int64 = string/);
  assert.match(typings, /executionTime\?: number/);
  assert.match(typings, /loginTime\?: number/);
  assert.match(typings, /totalOnlineUsers\?: number/);
  assert.match(typings, /type ResultInteger = Result<number>/);
});
