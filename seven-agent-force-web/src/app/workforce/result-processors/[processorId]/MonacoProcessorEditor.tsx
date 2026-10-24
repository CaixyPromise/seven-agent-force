import { useEffect, useRef } from 'react';
import * as monaco from 'monaco-editor/esm/vs/editor/editor.api';
import EditorWorker from 'monaco-editor/esm/vs/editor/editor.worker?worker';
import JsonWorker from 'monaco-editor/esm/vs/language/json/json.worker?worker';
import TsWorker from 'monaco-editor/esm/vs/language/typescript/ts.worker?worker';

self.MonacoEnvironment = {
  getWorker(_moduleId: string, label: string) {
    if (label === 'json') return new JsonWorker();
    if (label === 'javascript' || label === 'typescript') return new TsWorker();
    return new EditorWorker();
  },
};

const models = new Map<string, monaco.editor.ITextModel>();

export default function MonacoProcessorEditor({ processorId, path, value, readOnly = false, onChange }: {
  processorId: string;
  path: string;
  value: string;
  readOnly?: boolean;
  onChange: (value: string) => void;
}) {
  const container = useRef<HTMLDivElement>(null);
  const editor = useRef<monaco.editor.IStandaloneCodeEditor | undefined>(undefined);
  const changeHandler = useRef(onChange);
  const currentValue = useRef(value);
  changeHandler.current = onChange;
  currentValue.current = value;

  useEffect(() => {
    if (!container.current) return;
    editor.current = monaco.editor.create(container.current, {
      automaticLayout: true,
      minimap: { enabled: false },
      fontSize: 13,
      lineHeight: 21,
      scrollBeyondLastLine: false,
      padding: { top: 12 },
      accessibilityPageSize: 20,
      ariaLabel: 'Result Processor Python 脚本编辑器',
      theme: 'vs',
    });
    return () => editor.current?.dispose();
  }, []);

  useEffect(() => {
    const uri = monaco.Uri.parse(`inmemory://seven-result-processors/${encodeURIComponent(processorId)}/${path}`);
    let model = models.get(uri.toString());
    if (!model || model.isDisposed()) {
      model = monaco.editor.createModel(currentValue.current, language(path), uri);
      models.set(uri.toString(), model);
    } else if (model.getValue() !== currentValue.current) {
      model.setValue(currentValue.current);
    }
    editor.current?.setModel(model);
    const subscription = model.onDidChangeContent(() => changeHandler.current(model!.getValue()));
    return () => subscription.dispose();
  }, [processorId, path]);

  useEffect(() => {
    const model = editor.current?.getModel();
    if (model && model.getValue() !== value) model.setValue(value);
  }, [value]);
  useEffect(() => { editor.current?.updateOptions({ readOnly }); }, [readOnly]);
  useEffect(() => () => {
    for (const [key, model] of models) {
      if (key.includes(`/seven-result-processors/${encodeURIComponent(processorId)}/`)) {
        model.dispose();
        models.delete(key);
      }
    }
  }, [processorId]);

  return <div style={{ width: '100%', height: '100%', minWidth: 0 }} ref={container} />;
}

function language(path: string) {
  if (/\.py$/.test(path)) return 'python';
  if (/\.json$/.test(path)) return 'json';
  if (/\.md$/.test(path)) return 'markdown';
  return 'plaintext';
}
