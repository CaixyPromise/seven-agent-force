import { createSHA256 } from 'hash-wasm';
import { completeChunkUpload, initChunkUpload, uploadChunkPart } from '@/api/chunkUploadController';
import { uploadFile } from '@/api/fileController';
import { isAcceptedUploadResult } from '@/api/uploadContract';

const chunkSize = 5 * 1024 * 1024;
export async function sha256Blob(blob: Blob): Promise<string> {
  const hash = await createSHA256();
  for (let offset = 0; offset < blob.size; offset += chunkSize) hash.update(new Uint8Array(await blob.slice(offset, offset + chunkSize).arrayBuffer()));
  return hash.digest();
}
export async function uploadTaskInput(file: File, onProgress?: (value: number) => void): Promise<string> {
  if (file.size <= 10 * 1024 * 1024) {
    const result = await uploadFile(file);
    if (result.code !== 0 || !isAcceptedUploadResult(result.data)) throw new Error(result.message || '上传失败');
    onProgress?.(100); return result.data.fileId;
  }
  const sha256 = await sha256Blob(file);
  const initialized = await initChunkUpload({ fileName: file.name, fileSize: file.size, chunkSize, fileSha256: sha256, contentType: file.type });
  if (initialized.code !== 0 || !initialized.data) throw new Error(initialized.message || '初始化分片上传失败');
  for (let index = 0; index < initialized.data.totalChunks; index++) {
    const part = file.slice(index * chunkSize, Math.min(file.size, (index + 1) * chunkSize));
    const uploaded = await uploadChunkPart(initialized.data.uploadId, index + 1, part, await sha256Blob(part));
    if (uploaded.code !== 0 || !uploaded.data?.uploaded) throw new Error(`分片 ${index + 1} 上传失败`);
    onProgress?.(Math.round(((index + 1) / initialized.data.totalChunks) * 100));
  }
  const completed = await completeChunkUpload({ uploadId: initialized.data.uploadId });
  if (completed.code !== 0 || !isAcceptedUploadResult(completed.data)) throw new Error(completed.message || '完成上传失败');
  return completed.data.fileId;
}
export async function packDirectory(files: File[]): Promise<{ file: File; fileCount: number; excludedCount: number }> {
  return new Promise((resolve, reject) => {
    const worker = new Worker(new URL('./directoryPack.worker.ts', import.meta.url), { type: 'module' });
    worker.onmessage = (event: MessageEvent<{ archive?: Blob; fileCount?: number; excludedCount?: number; error?: string }>) => {
      worker.terminate();
      if (event.data.error || !event.data.archive) return reject(new Error('目录打包失败'));
      resolve({ file: new File([event.data.archive], 'workspace-directory.zip', { type: 'application/zip', lastModified: 315532800000 }), fileCount: event.data.fileCount ?? 0, excludedCount: event.data.excludedCount ?? 0 });
    };
    worker.onerror = (error) => { worker.terminate(); reject(error); };
    worker.postMessage({ files });
  });
}
