import { readFileSync } from 'node:fs';
import { fileURLToPath, URL } from 'node:url';
import { defineConfig, type Plugin } from 'vite';
import react from '@vitejs/plugin-react';
import { tanstackRouter } from '@tanstack/router-plugin/vite';
import wails from '@wailsio/runtime/plugins/vite';
import { themeTemplatesPlugin } from './build/theme-build';

function styleCatalogMock(): Plugin {
  const file = fileURLToPath(new URL('./mocks/style-catalog.json', import.meta.url));
  return {
    name: 'style-catalog-mock',
    configureServer(server) {
      server.middlewares.use('/mock/style-catalog.json', (_request, response) => {
        response.setHeader('Content-Type', 'application/json; charset=utf-8');
        response.setHeader('Cache-Control', 'no-store');
        response.end(readFileSync(file));
      });
    },
  };
}

export default defineConfig(({ command, mode }) => {
  const mock = process.env.WAILS_FRONTEND_MODE === 'mock';
  if (command === 'build' && mode === 'production' && mock) throw new Error('Production builds cannot enable the style catalog mock.');
  return {
    define: { __STYLE_CATALOG_MOCK__: JSON.stringify(mock) },
    plugins: [themeTemplatesPlugin(fileURLToPath(new URL('../themes', import.meta.url))), mock && styleCatalogMock(), { name: 'app-csp', transformIndexHtml: (html: string) => html.replace('__SCRIPT_SRC__', mode === 'production' ? "'self'" : "'self' 'unsafe-inline'").replace('__CONNECT_SRC__', mode === 'production' ? "'self'" : "'self' ws://127.0.0.1:* http://127.0.0.1:*") }, tanstackRouter({ target: 'react', autoCodeSplitting: false }), react(), wails('./bindings')],
    resolve: { alias: [
      { find: '@bindings', replacement: fileURLToPath(new URL('./bindings', import.meta.url)) },
    ] },
    server: { host: '127.0.0.1', port: Number(process.env.WAILS_VITE_PORT) || 9345, strictPort: true },
    build: { target: 'es2022', sourcemap: false },
  };
});
