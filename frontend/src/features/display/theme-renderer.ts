import Handlebars from 'handlebars/runtime';
import { Events } from '@wailsio/runtime';
import * as Display from '@bindings/token-monitor-turzx/internal/display/service';
import type { FrameRequest, ThemeData } from '@bindings/token-monitor-turzx/internal/display/models';
import { reportFrontendError } from '../application/queries';

type Manifest = {
  formatVersion: number;
  id: string;
  width: number;
  height: number;
  template: string;
  stylesheet: string;
};

const manifests = import.meta.glob<Manifest>('../../../../themes/*/theme.json', { eager: true, import: 'default' });
const specifications = import.meta.glob<Parameters<typeof Handlebars.template>[0]>('../../../../themes/*/*.hbs', { eager: true, import: 'default' });
const stylesheets = import.meta.glob<string>('../../../../themes/*/*.css', { eager: true, query: '?raw', import: 'default' });
const themes = new Map(Object.entries(manifests).map(([path, manifest]) => {
  const directory = path.slice(0, path.lastIndexOf('/') + 1);
  const specification = specifications[directory + manifest.template];
  const stylesheet = stylesheets[directory + manifest.stylesheet];
  return [manifest.id, { template: Handlebars.template<ThemeData>(specification), stylesheet }];
}));

function surface(width: number, height: number) {
  const canvas = document.createElement('canvas');
  canvas.width = width;
  canvas.height = height;
  const context = canvas.getContext('2d', { alpha: false });
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

  async function render(request: FrameRequest) {
    // Subscribe first so a failed initial asset request can be retried by the next frame.
    if (icons === undefined) {
      [icons] = await Promise.all([Display.AgentIcons(), document.fonts.load('500 22px "Yu Gothic"'), document.fonts.load('700 42px "Yu Gothic"')]);
    }
    const theme = themes.get(request.theme);
    if (!theme) throw new Error(`Unknown display theme: ${request.theme}`);
    // Bars keep the existing shrinking rule for large token totals.
    if (request.theme === 'bars') {
      for (const token of request.data.tokens!) {
        let size = 42;
        while (size > 20) {
          drawing.context.font = `700 ${size}px "Yu Gothic"`;
          if (drawing.context.measureText(token.tokensText).width <= 290) break;
          size -= 2;
        }
        token.tokensFontSize = size;
      }
    }
    const parsed = new DOMParser().parseFromString(theme.template(request.data), 'text/html');
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
    svg.setAttribute('width', '1920');
    svg.setAttribute('height', '462');
    svg.setAttribute('viewBox', '0 0 1920 462');
    const style = document.createElementNS(namespace, 'style');
    style.textContent = theme.stylesheet;
    const foreignObject = document.createElementNS(namespace, 'foreignObject');
    foreignObject.setAttribute('width', '1920');
    foreignObject.setAttribute('height', '462');
    foreignObject.append(document.importNode(parsed.body, true));
    svg.append(style, foreignObject);
    const image = new Image();
    image.src = 'data:image/svg+xml;charset=utf-8,' + encodeURIComponent(new XMLSerializer().serializeToString(svg));
    await image.decode();
    drawing.context.fillStyle = '#000';
    drawing.context.fillRect(0, 0, 1920, 462);
    drawing.context.drawImage(image, 0, 0);
    rotated.context.setTransform(1, 0, 0, 1, 0, 0);
    rotated.context.translate(462, 0);
    rotated.context.rotate(Math.PI / 2);
    rotated.context.drawImage(drawing.canvas, 0, 0);
    const png = encode(drawing.canvas, 'image/png');
    const jpeg = encode(rotated.canvas, 'image/jpeg');
    image.removeAttribute('src');
    await Display.CompleteFrame(request.id, png, jpeg, '');
  }

  async function pump() {
    requested = true;
    if (rendering) return;
    rendering = true;
    try {
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
  schedule();
  if (import.meta.hot) import.meta.hot.dispose(unsubscribe);
}
