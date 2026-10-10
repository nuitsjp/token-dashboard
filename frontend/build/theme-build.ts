import { readFileSync, readdirSync, statSync } from 'node:fs';
import { basename, dirname, extname, isAbsolute, join, relative, resolve, sep } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import Handlebars from 'handlebars';
import type { Plugin } from 'vite';
import { embedStylesheetImages as embedSheet, embedTemplateImages as embedTemplate } from './embed-theme';

type Manifest = {
  formatVersion: 1;
  id: string;
  name: string;
  width: 1920;
  height: 462;
  template: string;
  stylesheet: string;
};

type Theme = { manifest: Manifest; directory: string; templatePath: string; stylesheetPath: string };
type WatchAsset = (path: string) => void;

const imageTypes: Record<string, string> = {
  '.png': 'image/png', '.jpg': 'image/jpeg', '.jpeg': 'image/jpeg',
  '.gif': 'image/gif', '.webp': 'image/webp', '.avif': 'image/avif',
  '.svg': 'image/svg+xml', '.bmp': 'image/bmp', '.ico': 'image/x-icon',
};

function withinDirectory(directory: string, path: string) {
  const child = relative(directory, path);
  return child !== '..' && !child.startsWith(`..${sep}`) && !isAbsolute(child);
}

function themeFile(directory: string, value: unknown, extension: string) {
  // Runtime imports collect template and stylesheet files directly under each theme.
  if (typeof value !== 'string' || !value.trim() || basename(value) !== value || extname(value) !== extension) {
    throw new Error(`Theme file must be a ${extension} file directly inside ${directory}: ${String(value)}`);
  }
  const path = resolve(directory, value);
  if (!withinDirectory(directory, path) || !statSync(path).isFile()) throw new Error(`Invalid theme file: ${path}`);
  return path;
}

export function readThemeManifests(root: string): Theme[] {
  const ids = new Set<string>();
  return readdirSync(root, { withFileTypes: true })
    .filter(entry => entry.isDirectory() && !['node_modules', 'preview', 'benchmark'].includes(entry.name))
    .map(entry => {
      const directory = join(root, entry.name);
      const path = join(directory, 'theme.json');
      const manifest: Manifest = JSON.parse(readFileSync(path, 'utf8'));
      if (!manifest || typeof manifest !== 'object' || Array.isArray(manifest)
        || manifest.formatVersion !== 1 || manifest.width !== 1920 || manifest.height !== 462
        || typeof manifest.id !== 'string' || !manifest.id.trim()
        || typeof manifest.name !== 'string' || !manifest.name.trim()) {
        throw new Error(`Invalid display theme manifest: ${path}`);
      }
      if (ids.has(manifest.id)) throw new Error(`Duplicate display theme id: ${manifest.id}`);
      ids.add(manifest.id);
      return {
        manifest, directory,
        templatePath: themeFile(directory, manifest.template, '.hbs'),
        stylesheetPath: themeFile(directory, manifest.stylesheet, '.css'),
      };
    });
}

function embedImage(reference: string, sourcePath: string, watchAsset?: WatchAsset, embedCommonIcon = false) {
  if (reference.startsWith('#') || reference.includes('{{')) return reference;
  if (reference.startsWith('data:')) {
    if (!reference.startsWith('data:image/')) throw new Error(`Theme data URL must contain an image: ${sourcePath}`);
    return reference;
  }
  let sourceDirectory = dirname(sourcePath);
  let url: URL;
  if (reference.startsWith('/assets/agents/')) {
    if (!embedCommonIcon) return reference;
    sourceDirectory = resolve(import.meta.dirname, '../../internal/display/icons');
    url = new URL(reference.slice('/assets/agents/'.length), pathToFileURL(join(sourceDirectory, 'index')));
  } else {
    url = new URL(reference, pathToFileURL(sourcePath));
  }
  if (url.protocol !== 'file:') throw new Error(`Theme image must be local to its source file: ${reference} in ${sourcePath}`);
  const path = fileURLToPath(url);
  if (!withinDirectory(sourceDirectory, path)) {
    throw new Error(`Theme image must be local to its source file: ${reference} in ${sourcePath}`);
  }
  const contentType = imageTypes[extname(path).toLowerCase()];
  if (!contentType) throw new Error(`Unsupported theme image: ${reference} in ${sourcePath}`);
  watchAsset?.(path);
  return `data:${contentType};base64,${readFileSync(path).toString('base64')}${url.hash}`;
}

export function embedTemplateImages(source: string, sourcePath: string, watchAsset?: WatchAsset) {
  // Replace only parsed src attributes, preserving Handlebars blocks and all other HTML.
  return embedTemplate(source, sourcePath, (reference, path) => embedImage(reference, path, watchAsset));
}

export function embedStylesheetImages(source: string, sourcePath: string, watchAsset?: WatchAsset) {
  return embedSheet(source, sourcePath, (reference, path) => embedImage(reference, path, watchAsset, true));
}

export function themeTemplatesPlugin(root: string): Plugin {
  let themes: Theme[] = [];
  return {
    name: 'theme-templates',
    enforce: 'pre',
    buildStart() {
      themes = readThemeManifests(root);
      for (const theme of themes) {
        this.addWatchFile(join(theme.directory, 'theme.json'));
        this.addWatchFile(theme.templatePath);
        this.addWatchFile(theme.stylesheetPath);
        const watch = (path: string) => this.addWatchFile(path);
        embedTemplateImages(readFileSync(theme.templatePath, 'utf8'), theme.templatePath, watch);
        embedStylesheetImages(readFileSync(theme.stylesheetPath, 'utf8'), theme.stylesheetPath, watch);
      }
    },
    load(id) {
      const queryStart = id.indexOf('?');
      const path = resolve(queryStart < 0 ? id : id.slice(0, queryStart));
      const watch = (asset: string) => this.addWatchFile(asset);
      if (themes.some(theme => theme.templatePath === path)) {
        const source = embedTemplateImages(readFileSync(path, 'utf8'), path, watch);
        return `export default ${Handlebars.precompile(source, { strict: true })};`;
      }
      if (themes.some(theme => theme.stylesheetPath === path) && new URLSearchParams(id.slice(queryStart + 1)).has('raw')) {
        return `export default ${JSON.stringify(embedStylesheetImages(readFileSync(path, 'utf8'), path, watch))};`;
      }
    },
  };
}
