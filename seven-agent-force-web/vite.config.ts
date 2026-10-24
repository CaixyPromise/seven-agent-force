import path from 'node:path';
import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const frameworkApiTarget = env.VITE_DEV_API_TARGET?.trim() || 'http://127.0.0.1:9277';

  return {
    plugins: [react()],
    resolve: {
      alias: {
        '@': path.resolve(__dirname, 'src'),
        config: path.resolve(__dirname, 'config'),
      },
    },
    server: {
      host: '127.0.0.1',
      port: 5177,
      strictPort: true,
      proxy: {
        '/api': {
          target: frameworkApiTarget,
          changeOrigin: true,
        },
      },
    },
  };
});
