import { defineConfig } from 'vite';

export default defineConfig({
  cacheDir: '/tmp/airquality-vite',
  test: {
    cache: {
      dir: '/tmp/airquality-vitest',
    },
    include: ['src/__tests__/**/*.test.js'],
  },
});
