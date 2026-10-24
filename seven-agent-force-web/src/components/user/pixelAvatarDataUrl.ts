function hashSeed(seed: string) {
  let hash = 2166136261;
  for (let index = 0; index < seed.length; index += 1) {
    hash ^= seed.charCodeAt(index);
    hash = Math.imul(hash, 16777619);
  }
  return hash >>> 0;
}

export function pixelAvatarDataUrl(seed?: string) {
  const normalized = (seed || 'seven').trim() || 'seven';
  const hash = hashSeed(normalized);
  const hue = hash % 360;
  const accent = `hsl(${hue}, 72%, 48%)`;
  const secondary = `hsl(${(hue + 44) % 360}, 64%, 60%)`;
  const bg = `hsl(${(hue + 210) % 360}, 78%, 94%)`;
  const cells: string[] = [];
  for (let y = 0; y < 5; y += 1) {
    for (let x = 0; x < 3; x += 1) {
      const bit = (hash >> (y * 3 + x)) & 1;
      if (bit) {
        const color = (x + y) % 3 === 0 ? secondary : accent;
        cells.push(
          `<rect x="${x * 12 + 8}" y="${y * 12 + 8}" width="10" height="10" rx="2" fill="${color}"/>`,
        );
        if (x !== 2) {
          cells.push(
            `<rect x="${(4 - x) * 12 + 8}" y="${y * 12 + 8}" width="10" height="10" rx="2" fill="${color}"/>`,
          );
        }
      }
    }
  }
  const svg = `<svg xmlns="http://www.w3.org/2000/svg" width="72" height="72" viewBox="0 0 72 72"><rect width="72" height="72" rx="18" fill="${bg}"/><rect x="4" y="4" width="64" height="64" rx="16" fill="white" opacity=".45"/>${cells.join('')}</svg>`;
  return `data:image/svg+xml;utf8,${encodeURIComponent(svg)}`;
}
