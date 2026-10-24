const CONFIG_ASSET_PATH = /^\/api\/config-assets\/[1-9]\d*$/;

/**
 * The browser never treats a configuration value as a general URL. This is
 * intentionally stricter than same-origin URL parsing so query strings,
 * fragments, blob/data schemes and physical paths cannot become a preview or
 * runtime image source.
 */
export function isConfigAssetStablePath(value: unknown): value is string {
  return typeof value === 'string' && CONFIG_ASSET_PATH.test(value.trim());
}

export function configAssetStablePathOrEmpty(value: unknown): string {
  return isConfigAssetStablePath(value) ? value.trim() : '';
}
