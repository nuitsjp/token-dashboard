import Handlebars from 'handlebars/runtime';
import HandlebarsCompiler from 'handlebars';
import { Events } from '@wailsio/runtime';
import * as Display from '@bindings/token-monitor-turzx/internal/display/service';
import { invalidateStyleCatalog, loadStyleCatalog } from '../styles/catalog';
import type { FrameRequest, ThemeData } from '@bindings/token-monitor-turzx/internal/display/models';
import { reportFrontendError } from '../application/queries';

type Manifest = {
  formatVersion: number;
  id: string;
  width: number;
  height: number;
  dataLayout?: string;
  template: string;
  stylesheet: string;
};

const manifests = import.meta.glob<Manifest>(['../../../../themes/*/theme.json', '../../../../themes/services/*/theme.json'], { eager: true, import: 'default' });
const specifications = import.meta.glob<Parameters<typeof Handlebars.template>[0]>(['../../../../themes/*/*.hbs', '../../../../themes/services/*/*.hbs'], { eager: true, import: 'default' });
const stylesheets = import.meta.glob<string>(['../../../../themes/*/*.css', '../../../../themes/services/*/*.css'], { eager: true, query: '?raw', import: 'default' });
type LoadedTheme = { template: Handlebars.TemplateDelegate<ThemeData>; stylesheet: string; shrinkTokens: boolean; width: number; height: number };

const themes = new Map(Object.entries(manifests).map(([path, manifest]) => {
  const directory = path.slice(0, path.lastIndexOf('/') + 1);
  const specification = specifications[directory + manifest.template];
  const stylesheet = stylesheets[directory + manifest.stylesheet];
  return [manifest.id, { template: Handlebars.template<ThemeData>(specification), stylesheet, shrinkTokens: manifest.dataLayout === 'bars' || manifest.dataLayout?.startsWith('service-') === true, width: manifest.width, height: manifest.height }] as [string, LoadedTheme];
}));

export function registerTheme(id: string, template: string, stylesheet: string, width: number, height: number, dataLayout: string) {
  HandlebarsCompiler.parse(template);
  themes.set(id, {
    template: HandlebarsCompiler.compile<ThemeData>(template, { strict: true }),
    stylesheet,
    shrinkTokens: dataLayout === 'bars' || dataLayout.startsWith('service-') || template.includes('tokensFontSize'),
    width, height,
  });
}

export function unregisterTheme(id: string) { themes.delete(id); }

let gate = Promise.resolve();
function enqueue<T>(work: () => Promise<T>): Promise<T> {
  const result = gate.then(work, work);
  gate = result.then(() => undefined, () => undefined);
  return result;
}

let latest: ThemeData | undefined;
const themeDataListeners = new Set<() => void>();
export function subscribeThemeData(onChange: () => void) {
  themeDataListeners.add(onChange);
  return () => { themeDataListeners.delete(onChange); };
}

let renderPreview: ((themeId: string) => Promise<string>) | undefined;
export function renderStylePreview(themeId: string) {
  if (!renderPreview) return Promise.reject(new Error('Renderer is not started'));
  return renderPreview(themeId);
}

function surface(width: number, height: number) {
  const canvas = document.createElement('canvas');
  canvas.width = width;
  canvas.height = height;
  // Output reads pixels on every frame; keep the raster backend stable across readbacks.
  const context = canvas.getContext('2d', { alpha: false, willReadFrequently: true });
  if (!context) throw new Error('Canvas 2D is unavailable');
  return { canvas, context };
}

function encode(canvas: HTMLCanvasElement, type: 'image/png' | 'image/jpeg'): string {
  // WebView2 can delay toBlob callbacks by a second while the native window is hidden.
  const encoded = canvas.toDataURL(type, 0.85);
  const prefix = `data:${type};base64,`;
  if (!encoded.startsWith(prefix)) throw new Error(`Cannot encode ${type}`);
  return encoded.slice(prefix.length);
}

// The renderer lives outside React and keeps both Canvas surfaces for its lifetime.
export async function startThemeRenderer() {
  let icons: Awaited<ReturnType<typeof Display.AgentIcons>> | undefined;
  const drawing = surface(1920, 462);
  const rotated = surface(462, 1920);
  let rendering = false;
  let requested = false;
  let completedID = 0;

  async function draw(themeId: string, data: ThemeData, outputFormat?: string, shiftX = 0, shiftY = 0, reverse180 = false) {
    // Subscribe first so a failed initial asset request can be retried by the next frame.
    if (icons === undefined) {
      [icons] = await Promise.all([Display.AgentIcons(), document.fonts.load('500 22px "Yu Gothic"'), document.fonts.load('700 42px "Yu Gothic"')]);
    }
    const theme = themes.get(themeId);
    if (!theme) throw new Error(`Unknown display theme: ${themeId}`);
    const { width, height } = theme;
    if (drawing.canvas.width !== width || drawing.canvas.height !== height) { drawing.canvas.width = width; drawing.canvas.height = height; }
    const view = structuredClone(data);
    // Bars keep the existing shrinking rule for large token totals.
    if (theme.shrinkTokens) {
      const service = view.service;
      const tokens = service?.tokens ?? view.tokens ?? [];
      const available = service ? service.both && width > height ? (width - 24) * 0.42 - 20 : width - 44 : 290;
      for (const token of tokens) {
        let size = service ? service.both && width < height ? 20 : 24 : 42;
        while (size > (service ? 1 : 20)) {
          drawing.context.font = `700 ${size}px "Yu Gothic"`;
          if (Math.max(drawing.context.measureText(token.tokensText).width, service ? drawing.context.measureText(token.costText).width : 0) <= available) break;
          size -= service ? 1 : 2;
        }
        token.tokensFontSize = size;
      }
    }
    const parsed = new DOMParser().parseFromString(theme.template(view), 'text/html');
    for (const image of parsed.querySelectorAll('img')) {
      const source = image.getAttribute('src')!;
      if (source.startsWith('data:image/')) continue;
      if (!source.startsWith('/assets/agents/')) throw new Error(`Unresolved theme image: ${source}`);
      const embedded = icons![source];
      if (embedded === undefined) throw new Error(`Unknown Agent icon: ${source}`);
      image.setAttribute('src', embedded);
    }
    const namespace = 'http://www.w3.org/2000/svg';
    const svg = document.createElementNS(namespace, 'svg');
    svg.setAttribute('width', String(width));
    svg.setAttribute('height', String(height));
    svg.setAttribute('viewBox', `0 0 ${width} ${height}`);
    const style = document.createElementNS(namespace, 'style');
    style.textContent = theme.stylesheet;
    const foreignObject = document.createElementNS(namespace, 'foreignObject');
    foreignObject.setAttribute('width', String(width));
    foreignObject.setAttribute('height', String(height));
    foreignObject.append(document.importNode(parsed.body, true));
    svg.append(style, foreignObject);
    const image = new Image();
    image.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(new XMLSerializer().serializeToString(svg));
    await image.decode();
    drawing.context.fillStyle = '#000';
    drawing.context.fillRect(0, 0, width, height);
    drawing.context.drawImage(image, 0, 0);
    let jpeg = '';
    if (outputFormat) {
      const jpegOutput = outputFormat === 'jpeg';
      rotated.canvas.width = jpegOutput ? height : width;
      rotated.canvas.height = jpegOutput ? width : height;
      rotated.context.setTransform(1, 0, 0, 1, 0, 0);
      if (jpegOutput) { rotated.context.translate(height, 0); rotated.context.rotate(Math.PI / 2); }
      else if (reverse180) { rotated.context.translate(width, height); rotated.context.rotate(Math.PI); }
      // The gap the shift leaves is filled with the background, which is the colour of the image's corner.
      const [r, g, b] = drawing.context.getImageData(0, 0, 1, 1).data;
      rotated.context.fillStyle = `rgb(${r}, ${g}, ${b})`;
      rotated.context.fillRect(-width, -height, width * 3, height * 3);
      rotated.context.drawImage(drawing.canvas, shiftX, shiftY);
      jpeg = encode(rotated.canvas, jpegOutput ? 'image/jpeg' : 'image/png');
    }
    const png = encode(drawing.canvas, 'image/png');
    image.removeAttribute('src');
    return { png, jpeg };
  }

  async function render(request: FrameRequest) {
    const snapshot = structuredClone(request.data);
    latest = snapshot;
    for (const listener of themeDataListeners) listener();
    const frame = await enqueue(() => draw(request.theme, snapshot, request.outputFormat, request.shiftX, request.shiftY, request.reverse180));
    await Display.CompleteFrame(request.id, frame.png, frame.jpeg, '');
  }

  renderPreview = (themeId: string) => enqueue(async () => {
    if (!latest) throw new Error('No display image yet');
    const data = await Display.ThemePreview(themeId);
    const frame = await draw(themeId, data);
    return `data:image/png;base64,${frame.png}`;
  });

  async function pump() {
    requested = true;
    if (rendering) return;
    rendering = true;
    try {
      await loadStyleCatalog();
      while (requested) {
        requested = false;
        const request = await Display.RenderRequest();
        if (!request || request.id === completedID) continue;
        try {
          await render(request);
        } catch (error) {
          await Display.CompleteFrame(request.id, '', '', String(error));
        }
        completedID = request.id;
      }
    } finally {
      rendering = false;
    }
  }

  const schedule = () => { void pump().catch(reportFrontendError); };
  const unsubscribe = Events.On('display:render-requested', schedule);
  const catalogChanged = Events.On('styles:changed', () => { invalidateStyleCatalog(); schedule(); });
  schedule();
  if (import.meta.hot) import.meta.hot.dispose(() => { unsubscribe(); catalogChanged(); });
}
