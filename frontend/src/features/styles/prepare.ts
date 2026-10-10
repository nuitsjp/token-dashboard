import { embedStylesheetImages, embedTemplateImages } from '../../../build/embed-theme';

const imageTypes: Record<string, string> = {
  '.png': 'image/png', '.jpg': 'image/jpeg', '.jpeg': 'image/jpeg',
  '.gif': 'image/gif', '.webp': 'image/webp', '.avif': 'image/avif',
  '.svg': 'image/svg+xml', '.bmp': 'image/bmp', '.ico': 'image/x-icon',
};

export type ThemeFiles = { id: string; name: string; files: { [key: string]: string | undefined } | null };
export type PreparedTheme = { id: string; name: string; template: string; stylesheet: string };

function bytesOf(value: string) {
  const binary = atob(value);
  const bytes = new Uint8Array(binary.length);
  for (let index = 0; index < binary.length; index++) bytes[index] = binary.charCodeAt(index);
  return bytes;
}

function text(files: Map<string, Uint8Array>, name: string) {
  const data = files.get(name);
  if (!data) throw new Error(`Missing theme file: ${name}`);
  return new TextDecoder().decode(data);
}

function embed(reference: string, sourcePath: string, files: Map<string, Uint8Array>, icons: Record<string, string>, commonIcon: boolean) {
  if (reference.startsWith('#') || reference.includes('{{')) return reference;
  if (reference.startsWith('data:')) {
    if (!reference.startsWith('data:image/')) throw new Error(`Theme data URL must contain an image: ${sourcePath}`);
    return reference;
  }
  if (reference.startsWith('/assets/agents/')) {
    if (!commonIcon) return reference;
    const icon = icons[reference];
    if (!icon?.startsWith('data:image/')) throw new Error(`Theme image must be local to its source file: ${reference} in ${sourcePath}`);
    return icon;
  }
  const url = new URL(reference, `file:///theme/${sourcePath}`);
  if (url.protocol !== 'file:') throw new Error(`Theme image must be local to its source file: ${reference} in ${sourcePath}`);
  const path = decodeURIComponent(url.pathname).replace(/^\/theme\//, '');
  if (path.startsWith('../') || path.includes('/../') || path === '..') {
    throw new Error(`Theme image must be local to its source file: ${reference} in ${sourcePath}`);
  }
  const data = files.get(path);
  if (!data) throw new Error(`Unsupported theme image: ${reference} in ${sourcePath}`);
  const extension = path.slice(path.lastIndexOf('.')).toLowerCase();
  const contentType = imageTypes[extension];
  if (!contentType) throw new Error(`Unsupported theme image: ${reference} in ${sourcePath}`);
  let binary = '';
  for (const byte of data) binary += String.fromCharCode(byte);
  return `data:${contentType};base64,${btoa(binary)}${url.hash}`;
}

export function prepareTheme(value: ThemeFiles, icons: { [key: string]: string | undefined } | null): PreparedTheme {
  const present = Object.entries(value.files ?? {}).flatMap(([name, data]) => data === undefined ? [] : [[name, bytesOf(data)] as const]);
  const files = new Map(present);
  const known: Record<string, string> = {};
  for (const [name, data] of Object.entries(icons ?? {})) if (data !== undefined) known[name] = data;
  const manifest = JSON.parse(text(files, 'theme.json')) as { template?: string; stylesheet?: string };
  if (typeof manifest.template !== 'string' || typeof manifest.stylesheet !== 'string') throw new Error('Invalid display theme manifest');
  const template = embedTemplateImages(text(files, manifest.template), manifest.template, (reference, source) => embed(reference, source, files, known, false));
  const stylesheet = embedStylesheetImages(text(files, manifest.stylesheet), manifest.stylesheet, (reference, source) => embed(reference, source, files, known, true));
  return { id: value.id, name: value.name, template, stylesheet };
}
