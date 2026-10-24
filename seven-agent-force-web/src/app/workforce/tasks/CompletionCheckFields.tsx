import { lazy, Suspense, useId, useRef, useState } from 'react';
import { Button, Flex, Input, Radio, Select, Space, Spin, Typography, Upload } from 'antd';
import { PaperClipOutlined, UploadOutlined } from '@ant-design/icons';
import { completionCheckId, completionCheckLimits, completionCheckText, type CompletionCheck } from '@/api/completionCheckContract';
import { createReferenceAsset, type ReferenceAsset } from '@/api/workspaceInputController';
import { uploadTaskInput } from './taskInputUpload';

const MonacoHookEditor = lazy(() => import('../hooks/[hookId]/MonacoHookEditor'));

export default function CompletionCheckFields({ value, onChange, disabled = false, references = [], onBusyChange }: {
  value?: CompletionCheck; onChange?: (value: CompletionCheck) => void; disabled?: boolean; references?: ReferenceAsset[]; onBusyChange?: (busy: boolean) => void;
}) {
  const editorId = useId();
  const latest = useRef(value); latest.current = value;
  const [uploaded, setUploaded] = useState<ReferenceAsset[]>([]);
  const [uploading, setUploading] = useState(false);
  const busy = useRef(false);
  const [error, setError] = useState('');
  const mode = value?.mode ?? 'NONE';
  const update = (patch: Partial<CompletionCheck>) => onChange?.({ mode: 'NONE', ...latest.current, ...patch });
  const options = [...new Map([...references, ...uploaded].map((asset) => [asset.id, { value: asset.id, label: asset.displayName }])).values()];
  const importScript = async (file: File) => {
    if (busy.current) return false;
    busy.current = true; setError(''); setUploading(true); onBusyChange?.(true);
    try {
      if (file.size > completionCheckLimits.script) throw new Error();
      const script = new TextDecoder('utf-8', { fatal: true }).decode(await file.arrayBuffer());
      completionCheckText(script, 'script', completionCheckLimits.script, true);
      update({ script, mode: 'SCRIPT' });
    } catch { setError('脚本未导入，请选择不超过 64 KiB 的 UTF-8 文本文件。'); }
    finally { busy.current = false; setUploading(false); onBusyChange?.(false); }
    return false;
  };
  const addReference = async (file: File) => {
    if (busy.current) return false;
    busy.current = true; setError(''); setUploading(true); onBusyChange?.(true);
    try {
      if ((latest.current?.referenceAssetIds?.length ?? 0) >= 64) { setError('最多关联 64 份资料。'); return false; }
      const fileId = await uploadTaskInput(file);
      const asset = await createReferenceAsset(fileId, file.name);
      const id = completionCheckId(asset.id);
      setUploaded((current) => [...current, asset]);
      update({ referenceAssetIds: [...(latest.current?.referenceAssetIds ?? []), id] });
    } catch { setError('资料上传未完成，请重新选择文件重试。'); }
    finally { busy.current = false; setUploading(false); onBusyChange?.(false); }
    return false;
  };
  return <Space orientation="vertical" size={16} style={{ width: '100%' }}>
    <div><Typography.Text id={`${editorId}-mode`} strong>额外检查方式</Typography.Text><div style={{ marginTop: 8 }}><Radio.Group aria-labelledby={`${editorId}-mode`} disabled={disabled || uploading} value={mode} onChange={(event) => update({ mode: event.target.value })} options={[{ value: 'NONE', label: '无需额外命令' }, { value: 'COMMAND', label: '运行命令' }, { value: 'SCRIPT', label: '运行脚本' }]} /></div></div>
    {mode === 'COMMAND' && <div><label htmlFor={`${editorId}-command`}>检查命令</label><Input.TextArea id={`${editorId}-command`} disabled={disabled} value={value?.command ?? ''} onChange={(event) => update({ command: event.target.value })} rows={3} placeholder="例如 pnpm test" /></div>}
    {mode === 'SCRIPT' && <div><Flex align="center" justify="space-between" gap={8} wrap><Typography.Text strong>检查脚本</Typography.Text><Upload showUploadList={false} beforeUpload={importScript} disabled={disabled || uploading}><Button icon={<UploadOutlined />} loading={uploading} disabled={disabled}>导入脚本</Button></Upload></Flex><div style={{ height: 240, marginTop: 8, border: '1px solid var(--ant-color-border)', borderRadius: 6, overflow: 'hidden' }}><Suspense fallback={<Spin />}><MonacoHookEditor packageId={editorId} path="completion-check.sh" ariaLabel="完成检查脚本编辑器" value={value?.script ?? ''} readOnly={disabled || uploading} onChange={(script) => update({ script })} /></Suspense></div></div>}
    <div><label htmlFor={`${editorId}-instructions`}>检查说明</label><Input.TextArea id={`${editorId}-instructions`} disabled={disabled} value={value?.instructions ?? ''} onChange={(event) => update({ instructions: event.target.value })} rows={3} placeholder="说明预期结果和需要验证的条件" /></div>
    <div><label htmlFor={`${editorId}-references`}>检查参考资料</label><Select id={`${editorId}-references`} style={{ width: '100%' }} mode="multiple" maxCount={64} disabled={disabled || uploading} value={value?.referenceAssetIds ?? []} onChange={(referenceAssetIds) => update({ referenceAssetIds })} options={options} optionFilterProp="label" placeholder="选择已添加的资料" /><Upload showUploadList={false} beforeUpload={addReference} disabled={disabled || uploading}><Button style={{ marginTop: 8 }} icon={<PaperClipOutlined />} loading={uploading} disabled={disabled}>添加检查资料</Button></Upload></div>
    {error && <Typography.Text type="danger" role="alert">{error}</Typography.Text>}
  </Space>;
}
