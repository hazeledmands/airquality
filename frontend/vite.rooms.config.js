import { resolve } from 'path';
import { defineConfig } from 'vite';
import { viteSingleFile } from 'vite-plugin-singlefile';

export default defineConfig({
  cacheDir: '/tmp/airquality-vite-rooms',
  build: {
    outDir: 'dist-rooms',
    rollupOptions: {
      input: resolve(__dirname, 'rooms.html'),
    },
  },
  plugins: [viteSingleFile()],
});
