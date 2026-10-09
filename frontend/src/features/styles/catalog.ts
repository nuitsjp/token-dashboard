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

// Themes compiled into the app. Built-in cards are added by the list, so removing a built-in here does not drop it.
export function loadStyleCatalog(): Promise<StyleDefinition[]> {
  return Promise.resolve(definitions(Object.values(manifests)));
}
