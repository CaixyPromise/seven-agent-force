import assert from 'node:assert/strict';
import { access, readFile } from 'node:fs/promises';
import test from 'node:test';

async function readSource(path) {
  return readFile(new URL(path, import.meta.url), 'utf8');
}

const [apiSource, processorPageSource, menuSource] = await Promise.all([
  readSource('../src/api/resultProcessorController.ts'),
  readSource('../src/app/workforce/result-processors/page.tsx'),
  readSource('../src/lib/navigation/menuRoutes.tsx'),
]);

test('Result Processor authoring starts from a direct draft or archive', () => {
  assert.match(processorPageSource, /createResultProcessor\(createEmptyProcessorDraft/);
  assert.match(processorPageSource, /importResultProcessorArchive/);
  assert.doesNotMatch(processorPageSource, /ResultProcessorTemplate|templateId|从模板开始/);
  assert.match(apiSource, /seven_workforce_runtime\.processor import pass_result/);
});

test('template API and product route are removed', async () => {
  assert.doesNotMatch(apiSource, /ResultProcessorTemplate|result-processor-templates|createResultProcessorFromTemplate/);
  assert.doesNotMatch(menuSource, /result-processor-templates/);
  await assert.rejects(
    access(new URL('../src/app/workforce/result-processor-templates/page.tsx', import.meta.url)),
    (error) => error?.code === 'ENOENT',
  );
});
