import { fileURLToPath, URL } from 'node:url';
import { defineConfig } from 'vite';
import react from '@vitejs/plugin-react';
import { tanstackRouter } from '@tanstack/router-plugin/vite';
import wails from '@wailsio/runtime/plugins/vite';

export default defineConfig(({ mode }) => {
  return {
    plugins: [{ name: 'app-csp', transformIndexHtml: (html: string) => html.replace('__SCRIPT_SRC__', mode === 'production' ? "'self'" : "'self' 'unsafe-inline'").replace('__CONNECT_SRC__', mode === 'production' ? "'self'" : "'self' ws://127.0.0.1:* http://127.0.0.1:*") }, tanstackRouter({ target: 'react', autoCodeSplitting: false }), react(), wails('./bindings')],
    resolve: { alias: [
      { find: '@bindings', replacement: fileURLToPath(new URL('./bindings', import.meta.url)) },
    ] },
    server: { host: '127.0.0.1', port: Number(process.env.WAILS_VITE_PORT) || 9345, strictPort: true },
    build: { target: 'es2022', sourcemap: false },
  };
});
