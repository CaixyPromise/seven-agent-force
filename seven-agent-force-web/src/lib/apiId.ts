export type IdLike = string | number;

const maxSignedInt64 = '9223372036854775807';

export function hasId(value: unknown): value is IdLike {
  return value !== null && value !== undefined && String(value).trim().length > 0;
}

export function toIdString(value: unknown) {
  return String(value);
}

export function toApiIdParam(value: unknown): API.Int64 {
  if (typeof value === 'number') {
    if (!Number.isSafeInteger(value) || value <= 0) {
      throw new TypeError('Int64 identity must be supplied as a lossless decimal string');
    }
    return String(value);
  }
  const normalized = String(value).trim();
  if (!/^[1-9]\d*$/.test(normalized)) {
    throw new TypeError('Int64 identity must be a positive decimal string');
  }
	if (normalized.length > maxSignedInt64.length || (normalized.length === maxSignedInt64.length && normalized > maxSignedInt64)) {
		throw new TypeError('Int64 identity exceeds the signed 64-bit range');
	}
  return normalized;
}

export function toApiIdList(values: ReadonlyArray<unknown>): API.Int64[] {
  return values.filter(hasId).map((value) => toApiIdParam(value));
}
