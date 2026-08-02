// `vitest/config` rather than `vite`, because the `test` block below is Vitest's
// and vite's own defineConfig does not know about it. It re-exports everything
// vite's version does, so nothing else changes.
import { defineConfig } from 'vitest/config'
import react from '@vitejs/plugin-react'
import tailwindcss from '@tailwindcss/vite'

// https://vite.dev/config/
export default defineConfig({
  plugins: [react(), tailwindcss()],
  // `strictPort`, because the default is worse than a crash: Vite moves to 5174
  // *without failing* when 5173 is taken, and 5173 is the exact origin named by
  // CORS_ORIGINS in backend/.env. Every API call is then refused by CORS, which
  // surfaces as a signed-in app whose every screen is empty -- a symptom that
  // appears in no log of its own. Refusing to start says which port and why.
  server: { port: 5173, strictPort: true },
  test: {
    environment: 'jsdom',
    setupFiles: ['./src/test/setup.ts'],

    // Pinned here rather than inherited from `.env.local`, so the MSW handlers
    // and the app agree on one origin whatever a developer's env file says --
    // and so CI, which has no `.env.local`, resolves the same URLs.
    env: { VITE_API_BASE_URL: 'http://localhost:8080' },

    coverage: {
      provider: 'v8',
      reporter: ['text', 'lcov'],
      include: ['src/**/*.{ts,tsx}'],
      exclude: [
        'src/test/**',
        'src/main.tsx',
        'src/vite-env.d.ts',
        // Four lines of Firebase SDK wiring, replaced wholesale by the fake.
        'src/lib/firebase.ts',
      ],
      // §12.6's only frontend target: components at 60%. Scoped to that
      // directory rather than set globally, because a global floor would be a
      // number about the pages as well and §12.6 does not set one for them.
      thresholds: {
        'src/components/**': {
          lines: 60,
          functions: 60,
          statements: 60,
          branches: 60,
        },
      },
    },
  },
})
