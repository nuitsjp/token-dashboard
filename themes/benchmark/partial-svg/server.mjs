import { createServer } from "node:http";
import { readFile, readdir } from "node:fs/promises";
import Handlebars from "handlebars";
import { svgDisplayData } from "../svg/view-data.mjs";

const sample = JSON.parse(await readFile(new URL("../../preview/sample-data.json", import.meta.url), "utf8"));
const iconsRoot = new URL("../../../internal/display/icons/", import.meta.url);
const icons = new Map();
for (const name of await readdir(iconsRoot)) {
  const bytes = await readFile(new URL(name, iconsRoot));
  icons.set(name, `data:image/png;base64,${bytes.toString("base64")}`);
}
for (const contract of sample.contracts)
  contract.icon = icons.get(`${contract.provider.toLowerCase()}.png`) ?? null;

const templates = new Map();
for (const name of ["gauges", "bars"])
  templates.set(name, Handlebars.compile(await readFile(new URL(`../svg/${name}.svg.hbs`, import.meta.url), "utf8"), { strict: true }));

// The original templates escape all inserted strings. Matching structural <g>
// tags therefore cannot confuse an inserted label with markup.
function groupAt(svg, opening) {
  const start = svg.indexOf(opening);
  if (start < 0) throw new Error(`Missing SVG group: ${opening}`);
  const tags = /<g\b[^>]*>|<\/g>/g;
  tags.lastIndex = start;
  let depth = 0;
  for (let tag; (tag = tags.exec(svg));) {
    depth += tag[0].startsWith("</") ? -1 : 1;
    if (depth === 0) return svg.slice(start, tags.lastIndex);
  }
  throw new Error("Unclosed SVG group");
}

function patchDocument(svg, data, theme) {
  let x, y, width, height, group, defs = "", arrangement;
  if (theme === "gauges") {
    const panel = data.gaugePanels.find((item) => item.provider === "claude");
    ({ x, y, width, height } = panel);
    group = groupAt(svg, `<g transform="translate(${x} ${y})">`);
    arrangement = data.gaugePanels.map((item) => [item.provider, item.x, item.y, item.width, item.height, item.circles.map((circle) => circle.windows.map((window) => window.id))]);
  } else {
    const column = data.barColumns.find((item) => item.contracts.some((contract) => contract.provider === "claude"));
    const contract = column.contracts.find((item) => item.provider === "claude");
    x = column.x;
    y = column.y + contract.y;
    width = data.layout.bars.columnWidth;
    height = data.layout.bars.headerHeight + contract.windows.length * data.layout.bars.rowHeight;
    const columnGroup = groupAt(svg, `<g transform="translate(${column.x} ${column.y})">`);
    const contractGroup = groupAt(columnGroup, `<g transform="translate(0 ${contract.y})">`);
    group = `<g transform="translate(${column.x} ${column.y})">${contractGroup}</g>`;
    defs = svg.match(/<defs>[\s\S]*?<\/defs>/)[0];
    arrangement = data.barColumns.map((item) => [item.x, item.y, item.contracts.map((entry) => [entry.provider, entry.y, entry.windows.map((window) => window.id)])]);
  }
  const opening = svg.match(/^<svg\b[^>]*>/)[0]
    .replace(/\bwidth="[^"]*"/, `width="${width}"`)
    .replace(/\bheight="[^"]*"/, `height="${height}"`)
    .replace(/\bviewBox="[^"]*"/, `viewBox="${x} ${y} ${width} ${height}"`);
  const background = `<rect x="${x}" y="${y}" width="${width}" height="${height}" fill="${data.colors.background}" />`;
  return { svg: `${opening}${background}${defs}${group}</svg>`, x, y, width, height, layoutKey: JSON.stringify(arrangement) };
}

createServer((request, response) => {
  try {
    const url = new URL(request.url, "http://127.0.0.1");
    const theme = url.searchParams.get("theme");
    const template = templates.get(theme);
    const remaining = Number(url.searchParams.get("remaining"));
    if (!["/full", "/patch"].includes(url.pathname) || !template || !Number.isFinite(remaining) || remaining < 0 || remaining > 100)
      throw new Error("Expected /full or /patch, a known theme and remaining between 0 and 100");
    const started = performance.now();
    const data = svgDisplayData(sample, remaining);
    const full = template(data);
    const result = url.pathname === "/full" ? full : patchDocument(full, data, theme);
    const duration = performance.now() - started;
    response.writeHead(200, {
      "Content-Type": url.pathname === "/full" ? "image/svg+xml; charset=utf-8" : "application/json; charset=utf-8",
      "Cache-Control": "no-store",
      "Server-Timing": `theme;dur=${duration.toFixed(6)}`,
    });
    response.end(typeof result === "string" ? result : JSON.stringify(result));
  } catch (error) {
    response.writeHead(400, { "Content-Type": "text/plain; charset=utf-8" });
    response.end(error.message);
  }
}).listen(9351, "127.0.0.1", () => console.log("Partial SVG benchmark: http://127.0.0.1:9351/"));
