import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import path from 'node:path';

// KAI-307: Vite config for Kaivue web. Single build serves
// both /admin (customer admin) and /command (integrator portal)
// runtime contexts via React Router.
export default defineConfig({
  plugins: [react()],
  resolve: {
    alias: {
      '@': path.resolve(__dirname, './src'),
    },
  },
  server: {
    port: 5174,
    proxy: {
      '/api': {
        target: 'http://localhost:9997',
        changeOrigin: true,
      },
    },
  },
  build: {
    outDir: 'dist',
    sourcemap: true,
  },
});
