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

async function loadMockCatalog(): Promise<StyleDefinition[]> {
  shareTheme('night', 'bars');
  const response = await fetch('/mock/style-catalog.json');
  if (!response.ok) throw new Error('Style catalog is unavailable');
  return definitions(await response.json());
}

// The style list reads this function only. dev:mock replaces the result with the fixed catalog.
export function loadStyleCatalog(): Promise<StyleDefinition[]> {
  if (typeof __STYLE_CATALOG_MOCK__ !== 'undefined' && __STYLE_CATALOG_MOCK__) return loadMockCatalog();
  return Promise.resolve(definitions(Object.values(manifests)));
}

export const styleCatalogRefetchInterval = typeof __STYLE_CATALOG_MOCK__ !== 'undefined' && __STYLE_CATALOG_MOCK__ ? 500 : false;
