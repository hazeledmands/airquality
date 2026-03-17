import { defineConfig } from 'vite';
import { viteSingleFile } from 'vite-plugin-singlefile';

export default defineConfig({
  build: {
    outDir: 'dist',
  },
  plugins: [viteSingleFile()],
  test: {
    include: ['src/__tests__/**/*.test.js'],
  },
});
