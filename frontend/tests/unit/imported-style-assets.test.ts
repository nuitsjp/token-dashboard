import { readFileSync } from 'node:fs';
import { resolve } from 'node:path';
import { describe, expect, it } from 'vitest';
import { prepareTheme } from '../../src/features/styles/prepare';

describe('imported style image boundary', () => {
  const folder = resolve(import.meta.dirname, '../../../examples/styles/sample-paper-35-portrait');
  const files = Object.fromEntries(['theme.json', 'template.hbs', 'style.css', 'assets/badge.svg']
    .map(name => [name, readFileSync(resolve(folder, name)).toString('base64')]));
  const manifest = JSON.parse(readFileSync(resolve(folder, 'theme.json'), 'utf8'));

  it('embeds the copied Paper SVG in both HTML and CSS without its source folder', () => {
    const prepared = prepareTheme({ id: manifest.id, name: manifest.name, files }, {});
    const embedded = `data:image/svg+xml;base64,${files['assets/badge.svg']}`;
    expect(prepared.template).toContain(embedded);
    expect(prepared.stylesheet).toContain(embedded);
    expect(prepared.template).not.toContain('assets/badge.svg');
    expect(prepared.stylesheet).not.toContain('assets/badge.svg');
    expect(prepared).toMatchObject({ width: 320, height: 480, dataLayout: 'service-bars' });
  });

  it('rejects a missing copied image before the definition can be confirmed', () => {
    const missing = { ...files };
    delete missing['assets/badge.svg'];
    expect(() => prepareTheme({ id: manifest.id, name: manifest.name, files: missing }, {})).toThrow('badge.svg');
  });

  it('rejects a reference escaping the import folder', () => {
    const outside = { ...files, 'template.hbs': Buffer.from('<img src="../outside.svg">').toString('base64') };
    expect(() => prepareTheme({ id: manifest.id, name: manifest.name, files: outside }, {})).toThrow('outside.svg');
  });
});
