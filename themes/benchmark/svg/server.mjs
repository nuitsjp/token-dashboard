import { createServer } from "node:http";
import { readFile, readdir } from "node:fs/promises";
import { fileURLToPath } from "node:url";
import Handlebars from "handlebars";
import { svgDisplayData } from "./view-data.mjs";

const sample = JSON.parse(
  await readFile(
    new URL("../../preview/sample-data.json", import.meta.url),
    "utf8",
  ),
);
const icons = new Map();
const iconsRoot = new URL("../../../internal/display/icons/", import.meta.url);
// Icons are shared application resources, loaded once before measurement.
for (const name of await readdir(iconsRoot)) {
  const png = await readFile(new URL(name, iconsRoot));
  icons.set(name, `data:image/png;base64,${png.toString("base64")}`);
}
for (const contract of sample.contracts)
  contract.icon = icons.get(`${contract.provider.toLowerCase()}.png`) ?? null;
const templates = new Map();
for (const id of ["gauges", "bars"])
  templates.set(
    id,
    Handlebars.compile(
      await readFile(new URL(`${id}.svg.hbs`, import.meta.url), "utf8"),
      { strict: true },
    ),
  );

createServer((request, response) => {
  try {
    const url = new URL(request.url, "http://127.0.0.1");
    const template = templates.get(url.searchParams.get("theme"));
    const remaining = Number(url.searchParams.get("remaining"));
    if (
      !template ||
      !Number.isFinite(remaining) ||
      remaining < 0 ||
      remaining > 100
    )
      throw new Error("Expected a known theme and remaining between 0 and 100");
    const started = performance.now();
    const svg = template(svgDisplayData(sample, remaining));
    const generationMs = performance.now() - started;
    response.writeHead(200, {
      "Content-Type": "image/svg+xml; charset=utf-8",
      "Cache-Control": "no-store",
      "Server-Timing": `theme;dur=${generationMs.toFixed(6)}`,
    });
    response.end(svg);
  } catch (error) {
    response.writeHead(400, { "Content-Type": "text/plain; charset=utf-8" });
    response.end(error.message);
  }
}).listen(9350, "127.0.0.1", () =>
  console.log(
    `SVG benchmark: http://127.0.0.1:9350/ (${fileURLToPath(import.meta.url)})`,
  ),
);
