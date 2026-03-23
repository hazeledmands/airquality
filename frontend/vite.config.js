import { defineConfig } from 'vite';

export default defineConfig({
  cacheDir: '/tmp/airquality-vite',
  test: {
    cache: {
      dir: '/tmp/airquality-vitest',
    },
    include: ['src/__tests__/**/*.test.js'],
    coverage: {
      provider: 'v8',
      reporter: ['text', 'html'],
      include: ['src/**/*.js'],
      exclude: ['src/__tests__/**'],
      thresholds: { lines: 70, functions: 70 },
    },
  },
});
