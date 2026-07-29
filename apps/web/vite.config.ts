/// <reference types="vitest/config" />
import { defineConfig } from 'vite';
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
