// Axios normally parses JSON before application code can protect PostgreSQL
// BIGINT values. Workforce keeps internal IDs as decimal strings at the HTTP
// boundary; this compatibility transform also preserves a legacy bare int64
// response without ever routing it through JavaScript Number.
export function preserveLargeIntegerLiterals(data) {
  return data.replace(/(:\s*|,\s*|\[\s*)(-?\d{16,})(?=\s*[,}\]])/g, '$1"$2"');
}

export function parseJsonPreserveLargeIntegers(data, headers) {
  if (typeof data !== 'string') {
    return data;
  }

  const contentTypeHeader = headers?.['content-type'] ?? headers?.['Content-Type'] ?? '';
  const trimmed = data.trim();
  const looksLikeJson = trimmed.startsWith('{') || trimmed.startsWith('[');
  if (!contentTypeHeader.includes('application/json') && !looksLikeJson) {
    return data;
  }

  try {
    return JSON.parse(preserveLargeIntegerLiterals(data));
  } catch {
    return data;
  }
}
