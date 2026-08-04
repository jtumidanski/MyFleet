// `defineConfig` comes from vitest/config, not vite: as of Vite 8 the `test`
// key is no longer part of vite's own `UserConfig`, so importing it from 'vite'
// fails to typecheck. vitest/config re-exports vite's defineConfig widened with
// the `test` block.
import { defineConfig } from 'vitest/config';
import react from '@vitejs/plugin-react';

// Dev proxy: the SPA calls full `/api/<service>/...` paths; in dev we forward
// `/api` to the Traefik gateway on :80, which strips `/api/<service>` and routes
// to the matching backend service. baseUrl in the API client stays '' so the
// same paths work in prod (served behind the gateway).
export default defineConfig({
  plugins: [react()],
  server: {
    proxy: {
      '/api': {
        target: 'http://localhost',
        changeOrigin: true,
      },
    },
  },
  test: {
    globals: true,
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],
    css: false,
  },
});
