import { defineConfig } from 'vite'
import tailwindcss from '@tailwindcss/vite'

export default defineConfig({
  plugins: [tailwindcss()],
  build: {
    // Output next to the Go binary's static asset path.
    outDir: 'build/assets',
    rollupOptions: {
      input: 'src/app.css',
      output: {
        // Fixed name so app.html can reference it without a build manifest.
        assetFileNames: 'app.css',
        entryFileNames: '[name].js',
      },
    },
  },
})
