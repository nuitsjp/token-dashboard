import { readFileSync, readdirSync, statSync } from 'node:fs';
import { basename, dirname, extname, isAbsolute, join, relative, resolve, sep } from 'node:path';
import { fileURLToPath, pathToFileURL } from 'node:url';
import Handlebars from 'handlebars';
import { parseFragment } from 'parse5';
import { isTokenCloseParen, isTokenFunction, isTokenString, isTokenURL, isTokenWhiteSpaceOrComment, tokenize } from '@csstools/css-tokenizer';
import type { Plugin } from 'vite';

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
type Replacement = { start: number; end: number; value: string };
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

function replace(source: string, replacements: Replacement[]) {
  for (const item of replacements.sort((a, b) => b.start - a.start)) {
    source = source.slice(0, item.start) + item.value + source.slice(item.end);
  }
  return source;
}

export function embedTemplateImages(source: string, sourcePath: string, watchAsset?: WatchAsset) {
  const fragment = parseFragment(source, { sourceCodeLocationInfo: true });
  const replacements: Replacement[] = [];
  function visit(node: typeof fragment | (typeof fragment.childNodes)[number]) {
    if ('tagName' in node && node.tagName === 'img') {
      const src = node.attrs.find(attribute => attribute.name === 'src');
      const location = node.sourceCodeLocation?.attrs?.src;
      if (src && location) {
        const embedded = embedImage(src.value, sourcePath, watchAsset);
        if (embedded !== src.value) replacements.push({ start: location.startOffset, end: location.endOffset, value: `src="${embedded}"` });
      }
    }
    if ('childNodes' in node) node.childNodes.forEach(visit);
    if ('content' in node) visit(node.content);
  }
  visit(fragment);
  // Replace only parsed src attributes, preserving Handlebars blocks and all other HTML.
  return replace(source, replacements);
}

export function embedStylesheetImages(source: string, sourcePath: string, watchAsset?: WatchAsset) {
  const tokens = tokenize({ css: source }, { onParseError(error) { throw new Error(`Invalid theme CSS in ${sourcePath}: ${error.message}`); } });
  const replacements: Replacement[] = [];
  function image(reference: string) {
    if (reference.includes('{{')) throw new Error(`Dynamic CSS image URLs are unsupported: ${sourcePath}`);
    return embedImage(reference, sourcePath, watchAsset, true);
  }
  for (let index = 0; index < tokens.length; index++) {
    const token = tokens[index];
    if (isTokenURL(token)) {
      const embedded = image(token[4].value);
      if (embedded !== token[4].value) replacements.push({ start: token[2], end: token[3] + 1, value: `url("${embedded}")` });
    } else if (isTokenFunction(token) && token[4].value.toLowerCase() === 'url') {
      let valueIndex = index + 1;
      while (isTokenWhiteSpaceOrComment(tokens[valueIndex])) valueIndex++;
      const value = tokens[valueIndex];
      let closeIndex = valueIndex + 1;
      while (isTokenWhiteSpaceOrComment(tokens[closeIndex])) closeIndex++;
      if (!isTokenString(value) || !isTokenCloseParen(tokens[closeIndex])) throw new Error(`Invalid theme CSS url() in ${sourcePath}`);
      const embedded = image(value[4].value);
      if (embedded !== value[4].value) replacements.push({ start: value[2], end: value[3] + 1, value: `"${embedded}"` });
    }
  }
  return replace(source, replacements);
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
