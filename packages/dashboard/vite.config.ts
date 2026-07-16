import { defineConfig, loadEnv } from 'vite';
import preact from '@preact/preset-vite';

export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, '.', '');
  const target = env.LATEXMK_API_ORIGIN || 'http://127.0.0.1:8080';
  return {
    plugins: [preact()],
    server: {
      port: 4173,
      proxy: {
        '/v1': { target, changeOrigin: true },
        '/healthz': { target, changeOrigin: true },
        '/readyz': { target, changeOrigin: true },
      },
    },
  };
});
