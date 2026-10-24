function containsControlCharacter(value: string) {
  return Array.from(value).some((character) => {
    const codePoint = character.codePointAt(0) ?? 0;
    return codePoint <= 0x1f || codePoint === 0x7f;
  });
}

export function safeInternalPath(value?: string): value is string {
  if (
    !value
    || !value.startsWith('/')
    || value.startsWith('//')
    || value.includes('\\')
    || value.includes('%')
    || containsControlCharacter(value)
  ) {
    return false;
  }
  const path = value.split(/[?#]/, 1)[0];
  return !path.split('/').includes('..');
}

export function isTaskDeepLink(value?: string): value is string {
  if (!safeInternalPath(value)) return false;
  const path = value.split(/[?#]/, 1)[0];
  return /^\/workforce\/tasks\/[1-9]\d*$/.test(path);
}
