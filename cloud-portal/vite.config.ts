import { defineConfig, loadEnv } from 'vite';
import react from '@vitejs/plugin-react';

// In production, the portal is embedded in cloudbroker and served same-origin
// behind cloud.raikada.com (Cloudflare front). In dev, proxy API + websocket
// + relay routes to a local cloudbroker instance.
export default defineConfig(({ mode }) => {
  const env = loadEnv(mode, process.cwd(), '');
  const target = env.CLOUDBROKER_URL || 'http://localhost:8080';
  return {
    plugins: [react()],
    server: {
      port: 5180,
      proxy: {
        '/api': { target, changeOrigin: true },
        '/kaivue.v1': { target, changeOrigin: true },
        '/connect': { target, changeOrigin: true },
        '/relay': { target, ws: true, changeOrigin: true },
        '/stream': { target, changeOrigin: true },
        '/ws': { target, ws: true, changeOrigin: true },
      },
    },
  };
});
