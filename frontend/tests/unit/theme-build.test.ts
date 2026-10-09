import { mkdtempSync, mkdirSync, readFileSync, rmSync, writeFileSync } from 'node:fs';
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import { afterEach, beforeEach, describe, expect, it } from 'vitest';
import { build } from 'vite';
import { embedStylesheetImages, embedTemplateImages, readThemeManifests, themeTemplatesPlugin } from '../../build/theme-build';

const manifest = {
  formatVersion: 1, id: 'fixture', name: 'Fixture', width: 1920, height: 462,
  template: 'template.hbs', stylesheet: 'style.css',
};
const image = Buffer.from('fixture image bytes');
const embeddedImage = `data:image/png;base64,${image.toString('base64')}`;
let root: string;
let theme: string;

function writeManifest(value: unknown = manifest, directory = theme) {
  writeFileSync(join(directory, 'theme.json'), JSON.stringify(value));
}

beforeEach(() => {
  root = mkdtempSync(join(tmpdir(), 'token-dashboard-theme-'));
  theme = join(root, 'fixture');
  mkdirSync(join(theme, 'assets'), { recursive: true });
  writeManifest();
  writeFileSync(join(theme, 'template.hbs'), '<main>{{label}}</main>');
  writeFileSync(join(theme, 'style.css'), 'main { color: #fff; }');
  writeFileSync(join(theme, 'assets', 'image.png'), image);
});

afterEach(() => rmSync(root, { recursive: true, force: true }));

describe('theme image embedding', () => {
  it('resolves parsed HTML src attributes and preserves Handlebars, comments, embedded images and Agent icons', () => {
    writeFileSync(join(theme, 'assets', 'a&b.png'), image);
    const source = `{{#if available}}<IMG SRC='./assets/a&amp;b.png' alt='x > y'>{{/if}}
      <!-- <img src='./missing.png'> -->
      <img src=${'./assets/image.png'}><img src="{{icon}}">
      <img src="data:image/png;base64,AAAA"><img src="/assets/agents/claude.png">`;
    const result = embedTemplateImages(source, join(theme, 'template.hbs'));
    expect(result).toContain(`{{#if available}}<IMG src="${embeddedImage}" alt='x > y'>{{/if}}`);
    expect(result).toContain(`<!-- <img src='./missing.png'> -->`);
    expect(result).toContain(`<img src="${embeddedImage}"><img src="{{icon}}">`);
    expect(result).toContain('<img src="data:image/png;base64,AAAA">');
    expect(result).toContain('<img src="/assets/agents/claude.png">');
  });

  it('resolves CSS URL tokens, including escaped paths, and leaves text and comments unchanged', () => {
    writeFileSync(join(theme, 'assets', 'image one.png'), image);
    const source = String.raw`/* url('./missing.png') */
      main { background: URL( './assets/image.png' /*keep*/ );
        mask: url(./assets/image\ one.png); content: "url('./missing.png')";
        border-image: url("data:image/png;base64,AAAA");
        list-style: url('/assets/agents/claude.png'); filter: url(#filter); }`;
    const result = embedStylesheetImages(source, join(theme, 'style.css'));
    expect(result).toContain(`URL( "${embeddedImage}" /*keep*/ )`);
    expect(result).toContain(`mask: url("${embeddedImage}")`);
    expect(result).toContain(`content: "url('./missing.png')"`);
    expect(result).toContain(`/* url('./missing.png') */`);
    expect(result).toContain('url("data:image/png;base64,AAAA")');
    const commonIcon = readFileSync(join(import.meta.dirname, '../../../internal/display/icons/claude.png')).toString('base64');
    expect(result).toContain(`url("data:image/png;base64,${commonIcon}")`);
    expect(result).toContain('url(#filter)');
  });

  it('fails for missing HTML or CSS images', () => {
    expect(() => embedTemplateImages('<img src="./assets/missing.png">', join(theme, 'template.hbs'))).toThrow('missing.png');
    expect(() => embedStylesheetImages('main { background: url("./assets/missing.png"); }', join(theme, 'style.css'))).toThrow('missing.png');
  });

  it('rejects unsupported dynamic CSS URLs and malformed quoted URLs', () => {
    expect(() => embedStylesheetImages('main { background: url("{{icon}}"); }', join(theme, 'style.css'))).toThrow('Dynamic CSS image URLs are unsupported');
    expect(() => embedStylesheetImages('main { background: url("./assets/image.png"', join(theme, 'style.css'))).toThrow('Invalid theme CSS url()');
  });

  it('rejects non-image data URLs and external images', () => {
    expect(() => embedTemplateImages('<img src="data:text/plain,hello">', join(theme, 'template.hbs'))).toThrow('Theme data URL must contain an image');
    expect(() => embedStylesheetImages('main { background: url("data:text/plain,hello"); }', join(theme, 'style.css'))).toThrow('Theme data URL must contain an image');
    expect(() => embedTemplateImages('<img src="https://example.invalid/image.png">', join(theme, 'template.hbs'))).toThrow('Theme image must be local');
  });
});

describe('theme manifest validation', () => {
  it.each([
    null,
    [],
    { ...manifest, formatVersion: 2 },
    { ...manifest, width: 1280 },
    { ...manifest, height: '462' },
    { ...manifest, id: '' },
    { ...manifest, name: 12 },
    { ...manifest, template: '../template.hbs' },
    { ...manifest, stylesheet: 'style.txt' },
  ])('rejects invalid manifest %j', value => {
    writeManifest(value);
    expect(() => readThemeManifests(root)).toThrow();
  });

  it('fails for missing referenced files', () => {
    writeManifest({ ...manifest, stylesheet: 'missing.css' });
    expect(() => readThemeManifests(root)).toThrow('missing.css');
  });

  it('rejects duplicate IDs across theme directories', () => {
    const second = join(root, 'second');
    mkdirSync(second);
    writeManifest(manifest, second);
    writeFileSync(join(second, 'template.hbs'), '');
    writeFileSync(join(second, 'style.css'), '');
    expect(() => readThemeManifests(root)).toThrow('Duplicate display theme id: fixture');
  });
});

describe('Vite theme build boundary', () => {
  async function bundle() {
    writeFileSync(join(root, 'main.js'), `import template from './fixture/template.hbs';
      import stylesheet from './fixture/style.css?raw'; export { template, stylesheet };`);
    return build({
      configFile: false, root, logLevel: 'silent', plugins: [themeTemplatesPlugin(root)],
      build: { write: false, minify: false, lib: { entry: join(root, 'main.js'), formats: ['es'] } },
    });
  }

  it('embeds both images into the built template specification and raw stylesheet', async () => {
    writeFileSync(join(theme, 'template.hbs'), '<img src="./assets/image.png">');
    writeFileSync(join(theme, 'style.css'), 'main { background: url("./assets/image.png"); }');
    const result = await bundle();
    const outputs = (Array.isArray(result) ? result : [result]).flatMap(result => 'output' in result ? result.output : []);
    const code = outputs.filter(output => output.type === 'chunk').map(output => output.code).join('\n');
    expect(code.split(embeddedImage)).toHaveLength(3);
    expect(code).not.toContain('./assets/image.png');
  });

  it('fails the build when a manifest is invalid before rendering', async () => {
    writeManifest({ ...manifest, formatVersion: 2 });
    await expect(bundle()).rejects.toThrow('Invalid display theme manifest');
  });

  it('fails the build when an image is missing', async () => {
    writeFileSync(join(theme, 'style.css'), 'main { background: url("./assets/missing.png"); }');
    await expect(bundle()).rejects.toThrow('missing.png');
  });

  it('keeps the existing themes unchanged after embedding', () => {
    const existingRoot = join(import.meta.dirname, '../../../themes');
    for (const theme of readThemeManifests(existingRoot)) {
      const template = readFileSync(theme.templatePath, 'utf8');
      const stylesheet = readFileSync(theme.stylesheetPath, 'utf8');
      expect(embedTemplateImages(template, theme.templatePath)).toBe(template);
      expect(embedStylesheetImages(stylesheet, theme.stylesheetPath)).toBe(stylesheet);
    }
  });
});
