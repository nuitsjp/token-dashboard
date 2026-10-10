import { shareTheme } from '../display/theme-renderer';

export type StyleDefinition = { id: string; name: string };

type Manifest = { id?: string; name?: string };

const manifests = import.meta.glob<Manifest>('../../../../themes/*/theme.json', { eager: true, import: 'default' });

function definitions(value: unknown): StyleDefinition[] {
  if (!Array.isArray(value)) throw new Error('Style catalog is unavailable');
  const seen = new Set<string>();
  const list: StyleDefinition[] = [];
  for (const item of value) {
    if (!item || typeof item !== 'object') continue;
    const id = 'id' in item && typeof item.id === 'string' ? item.id.trim() : '';
    const name = 'name' in item && typeof item.name === 'string' ? item.name.trim() : '';
    if (!id || !name || seen.has(id)) continue;
    seen.add(id);
    list.push({ id, name });
  }
  return list;
}

const imported: StyleDefinition[] = [];

function compiledStyles(): StyleDefinition[] {
  return definitions(Object.values(manifests));
}

// Themes compiled into the app, plus styles imported in this session.
// Built-in cards are added by the list, so removing a built-in here does not drop it.
export function loadStyleCatalog(): Promise<StyleDefinition[]> {
  const compiled = compiledStyles();
  const seen = new Set(compiled.map(style => style.id));
  return Promise.resolve([...compiled, ...imported.filter(style => !seen.has(style.id))]);
}

// dev:mock replaces folder inspection and copying with the fixed definition.
export async function importStyleFolder(): Promise<void> {
  if (typeof __STYLE_ADD_MOCK__ === 'undefined' || !__STYLE_ADD_MOCK__) throw new Error('Style import is unavailable');
  const response = await fetch('/mock/added-style.json');
  if (!response.ok) throw new Error('Style catalog is unavailable');
  const [style] = definitions([await response.json()]);
  if (!style) throw new Error('Style catalog is unavailable');
  const current = [...compiledStyles(), ...imported];
  if (current.some(item => item.id === style.id)) throw { code: 'DUPLICATE', message: 'This style id is already defined.' };
  shareTheme(style.id, 'bars');
  imported.push(style);
}
