import { createServer } from "node:http";
import { readFile, readdir } from "node:fs/promises";
import { extname, join, relative, resolve, sep } from "node:path";
import { fileURLToPath } from "node:url";
import Handlebars from "handlebars";
import { embedStylesheetImages, embedTemplateImages, readThemeManifests } from "../../frontend/build/theme-build.ts";

const root = fileURLToPath(new URL("../", import.meta.url));
const iconsRoot = fileURLToPath(
  new URL("../../internal/display/icons/", import.meta.url),
);
const iconFiles = new Set(await readdir(iconsRoot));
const sample = JSON.parse(
  await readFile(join(root, "preview/sample-data.json"), "utf8"),
);
const themes = new Map();
for (const { manifest, templatePath, stylesheetPath } of readThemeManifests(root)) {
  const template = Handlebars.compile(
    embedTemplateImages(await readFile(templatePath, "utf8"), templatePath),
    { strict: true },
  );
  const stylesheet = embedStylesheetImages(await readFile(stylesheetPath, "utf8"), stylesheetPath);
  themes.set(manifest.id, { manifest, template, stylesheet });
}

// This preview consumes prepared display data; it does not acquire or calculate usage.
function displayData(params) {
  const data = structuredClone(sample);
  data.available = params.get("state") !== "waiting";
  const changedId = params.get("window");
  const changedValue = params.get("remaining");
  const unknown = params.get("state") === "unknown";
  for (const contract of data.contracts) {
    const icon = `${contract.provider.toLowerCase()}.png`;
    contract.icon = iconFiles.has(icon) ? `/assets/agents/${icon}` : null;
    for (const circle of contract.circles) {
      circle.double = circle.windows.length === 2;
      circle.showName = contract.circles.some(
        (item) => item.name !== circle.name,
      );
      for (const window of circle.windows) {
        if (window.id === changedId && changedValue !== null) {
          const value = Number(changedValue);
          if (!Number.isFinite(value) || value < 0 || value > 100)
            throw new Error("残量は0〜100で指定してください。");
          window.remainingPercent = unknown ? null : value;
          // Demo edits show the remaining-level color; the supplied fixture already contains pace colors.
          window.tone =
            value < 25 ? "danger" : value <= 40 ? "warning" : "normal";
        }
        window.hasRemaining = window.remainingPercent !== null;
        window.percentText = window.hasRemaining
          ? `${Math.round(window.remainingPercent)}%`
          : "—";
      }
    }
    contract.span = contract.circles.length;
    contract.windows = contract.circles
      .flatMap((circle) => circle.windows)
      .slice(0, 4);
  }
  const lowest = (windows) =>
    Math.min(
      ...windows
        .filter((window) => window.hasRemaining)
        .map((window) => window.remainingPercent),
    );
  const sorted = data.contracts.toSorted(
    (a, b) =>
      lowest(a.circles.flatMap((circle) => circle.windows)) -
      lowest(b.circles.flatMap((circle) => circle.windows)),
  );
  let usedColumns = 0;
  data.gaugePanels = sorted.filter((contract) => {
    if (usedColumns + contract.span > 7) return false;
    usedColumns += contract.span;
    return true;
  });
  data.barColumns = [];
  for (const contract of data.contracts.toSorted(
    (a, b) => lowest(a.windows) - lowest(b.windows),
  )) {
    const column = data.barColumns.find(
      (item) =>
        item.contracts.length < 2 &&
        item.contracts.reduce(
          (count, existing) => count + existing.windows.length,
          0,
        ) +
          contract.windows.length <=
          4,
    );
    if (column) column.contracts.push(contract);
    else if (data.barColumns.length < 4)
      data.barColumns.push({ contracts: [contract] });
  }
  return data;
}

const types = {
  ".html": "text/html; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".json": "application/json; charset=utf-8",
  ".png": "image/png",
};
const server = createServer(async (request, response) => {
  try {
    const url = new URL(request.url, "http://127.0.0.1");
    if (url.pathname === "/render") {
      const theme = themes.get(url.searchParams.get("theme"));
      if (!theme) {
        response.writeHead(404);
        response.end("Unknown theme");
        return;
      }
      const { manifest, template, stylesheet } = theme;
      const started = performance.now();
      const body = template(displayData(url.searchParams));
      const generationMs = performance.now() - started;
      response.writeHead(200, {
        "Content-Type": types[".html"],
        "Cache-Control": "no-store",
        "Server-Timing": `theme;dur=${generationMs.toFixed(6)}`,
      });
      response.end(
        `<!doctype html><html lang="en"><head><meta charset="utf-8"><link rel="icon" href="data:,"><title>${manifest.name}</title><style>${stylesheet}</style></head><body>${body}</body></html>`,
      );
      return;
    }
    if (url.pathname.startsWith("/assets/agents/")) {
      const name = url.pathname.slice("/assets/agents/".length);
      if (!iconFiles.has(name)) {
        response.writeHead(404);
        response.end("Unknown agent icon");
        return;
      }
      const body = await readFile(join(iconsRoot, name));
      response.writeHead(200, {
        "Content-Type": "image/png",
        "Cache-Control": "public, max-age=3600",
      });
      response.end(body);
      return;
    }
    if (url.pathname === "/themes") {
      response.writeHead(200, { "Content-Type": types[".json"] });
      response.end(
        JSON.stringify([...themes.values()].map(({ manifest }) => manifest)),
      );
      return;
    }
    const path = resolve(
      root,
      `.${decodeURIComponent(url.pathname === "/" ? "/preview/index.html" : url.pathname)}`,
    );
    const within = relative(root, path);
    if (
      within.startsWith(`..${sep}`) ||
      within === ".." ||
      within.split(sep).includes("node_modules")
    ) {
      response.writeHead(403);
      response.end("Forbidden");
      return;
    }
    const body = await readFile(path);
    response.writeHead(200, {
      "Content-Type": types[extname(path)] ?? "text/plain; charset=utf-8",
    });
    response.end(body);
  } catch (error) {
    response.writeHead(error.code === "ENOENT" ? 404 : 400, {
      "Content-Type": "text/plain; charset=utf-8",
    });
    response.end(error.message);
  }
});
server.listen(9349, "127.0.0.1", () =>
  console.log("Theme preview: http://127.0.0.1:9349/"),
);
