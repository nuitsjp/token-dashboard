import * as Application from '@bindings/token-monitor-turzx/internal/desktop/service';
import * as Display from '@bindings/token-monitor-turzx/internal/display/service';
import * as Styles from '@bindings/token-monitor-turzx/internal/styles/service';
import { registerTheme, unregisterTheme } from '../display/theme-renderer';
import { prepareTheme, type ThemeFiles } from './prepare';

export type StyleDefinition = { id: string; name: string };

type Manifest = { id?: string; name?: string };

const manifests = import.meta.glob<Manifest>(['../../../../themes/*/theme.json', '../../../../themes/services/*/theme.json'], { eager: true, import: 'default' });
const invalid = { code: 'INVALID', message: 'This folder is not a valid style.' };
let loading: Promise<StyleDefinition[]> | undefined;
let revision = 0;
let registered = new Set<string>();
export function invalidateStyleCatalog() { revision++; loading = undefined; }

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

export function compiledStyles(): StyleDefinition[] {
  return definitions(Object.values(manifests));
}

function isFault(error: unknown): boolean {
  return typeof error === 'object' && error !== null && 'code' in error && 'message' in error;
}

async function registerStored(value: ThemeFiles) {
  const icons = await Display.AgentIcons();
  const prepared = prepareTheme(value, icons);
  registerTheme(prepared.id, prepared.template, prepared.stylesheet, prepared.width, prepared.height, prepared.dataLayout);
  return { id: prepared.id, name: prepared.name };
}

export async function deleteStyle(id: string): Promise<void> {
  await Styles.Delete(id);
  unregisterTheme(id);
  invalidateStyleCatalog();
}

export function loadStyleCatalog(): Promise<StyleDefinition[]> {
  if (!loading) {
    const current = revision;
    loading = readStyleCatalog(current).catch(error => { if (current === revision) loading = undefined; throw error; });
  }
  return loading;
}

async function readStyleCatalog(current: number): Promise<StyleDefinition[]> {
  const compiled = compiledStyles();
  const stored = (await Styles.List()) ?? [];
  const icons = stored.length ? await Display.AgentIcons() : {};
  const prepared = stored.map(style => prepareTheme(style, icons));
  if (current !== revision) return loadStyleCatalog();
  const next = new Set(prepared.map(style => style.id));
  for (const id of registered) if (!next.has(id)) unregisterTheme(id);
  for (const style of prepared) registerTheme(style.id, style.template, style.stylesheet, style.width, style.height, style.dataLayout);
  registered = next;
  const seen = new Set(compiled.map(style => style.id));
  return [...compiled, ...prepared.filter(style => !seen.has(style.id)).map(({ id, name }) => ({ id, name }))];
}

export async function importStyleFolder(): Promise<void> {
  const folder = await Application.ChooseStyleFolder();
  if (!folder) return;
  const draft = await Styles.BeginImport(folder, compiledStyles().map(style => style.id));
  try {
    await registerStored(draft);
    await Styles.FinishImport(draft.token);
    invalidateStyleCatalog();
  } catch (error) {
    await Styles.CancelImport(draft.token);
    unregisterTheme(draft.id);
    if (isFault(error)) throw error;
    throw invalid;
  }
}
