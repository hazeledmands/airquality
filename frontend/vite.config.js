import { defineConfig } from 'vite';
import { viteSingleFile } from 'vite-plugin-singlefile';

export default defineConfig({
  cacheDir: '/tmp/airquality-vite',
  build: {
    outDir: 'dist',
  },
  plugins: [viteSingleFile()],
  test: {
    cache: {
      dir: '/tmp/airquality-vitest',
    },
    include: ['src/__tests__/**/*.test.js'],
  },
});
