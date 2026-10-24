export interface FileCheckData {
  exists: boolean;
  fileId?: string;
}

export interface FasterUploadInput {
  fileName: string;
  contentType?: string;
  sha256: string;
  fileSize: number;
}

export interface UploadResultData {
  fileId?: string;
}

export function isExistingFile(data?: FileCheckData): boolean {
  return data?.exists === true && data.fileId !== undefined;
}

export function buildFasterUploadInput(input: FasterUploadInput): FasterUploadInput {
  return {
    fileName: input.fileName,
    contentType: input.contentType,
    sha256: input.sha256,
    fileSize: input.fileSize,
  };
}

export function isAcceptedUploadResult(
  data?: UploadResultData,
): data is UploadResultData & { fileId: string } {
  return Boolean(
    data
    && typeof data.fileId === 'string'
    && /^\d+$/.test(data.fileId),
  );
}
