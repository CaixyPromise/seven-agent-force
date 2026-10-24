import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { fileURLToPath } from 'node:url';
import path from 'node:path';
const root = fileURLToPath(new URL('.', import.meta.url));
export default defineConfig({
  root, plugins: [react()],
  resolve: { alias: [
    ...['conversationController', 'executionAccessController', 'deliveryController'].map((name) => ({ find: `@/api/${name}`, replacement: path.join(root, 'fixtures.ts') })),
    { find: '@', replacement: path.resolve(root, '../../src') },
  ] },
  server: { host: '127.0.0.1', port: 5178, strictPort: true, fs: { allow: [path.resolve(root, '../..')] } },
});
