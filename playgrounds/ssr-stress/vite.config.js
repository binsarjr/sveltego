import { svelte } from '@sveltejs/vite-plugin-svelte';

/** @type {import('vite').UserConfig} */
export default {
  plugins: [svelte()],
  build: {
    outDir: 'static/_app',
    manifest: true,
  },
};
