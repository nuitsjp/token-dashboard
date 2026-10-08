// SVG geometry belongs to the benchmark theme; it is not a product data contract.
export const svgLayout = {
  width: 1920,
  height: 462,
  gauges: {
    columns: 7,
    margin: 8,
    panelGap: 14,
    panelTop: 86,
    panelBottom: 30,
    panelRadius: 16,
    headingX: 14,
    headingBaseline: 38,
    iconSize: 28,
    iconGap: 10,
    circleSize: 206.36,
    circleTop: 52,
    outerRadius: 90,
    innerRadius: 72,
    strokeWidth: 11,
    resetBaseline: 289,
    resetStep: 35,
    resetFontSize: 22,
    resetLabelWidth: 36,
    resetLabelGap: 13,
    clockSize: 18,
    resetTextGap: 9,
    tokenX: 15,
    tokenBaseline: 64,
    tokenGap: 64,
    tokenItemGap: 14,
  },
  bars: {
    tokensLeft: 40,
    tokensRight: 330,
    tokensTop: 30,
    tokensStep: 140,
    dividerX: 356,
    dividerY: 30,
    dividerHeight: 402,
    columns: 4,
    columnLeft: 384,
    columnTop: 26,
    columnWidth: 353,
    columnGap: 28,
    headingBaseline: 33,
    headerHeight: 54,
    rowHeight: 72,
    rowBaseline: 25,
    barY: 37,
    barHeight: 14,
    resetRight: 263,
    percentRight: 353,
    stackGap: 18,
    maxRows: 4,
    maxContracts: 2,
  },
};

const colors = {
  background: "#0f1117",
  panel: "#151821",
  divider: "#2a2f3a",
  text: "#e8eaf0",
  dim: "#8a90a0",
  accent: "#7cc4ff",
  normal: "#7466e0",
  warning: "#fab219",
  danger: "#f0616d",
};

function arc(radius, percent) {
  const start = (135 * Math.PI) / 180;
  const sweep = (270 * Math.max(0, Math.min(100, percent))) / 100;
  const end = start + (sweep * Math.PI) / 180;
  const point = (angle) =>
    `${(100 + radius * Math.cos(angle)).toFixed(4)} ${(100 + radius * Math.sin(angle)).toFixed(4)}`;
  const from = point(start);
  // A zero-length round-capped line preserves the zero-percent marker.
  if (sweep === 0) return `M ${from} L ${from}`;
  return `M ${from} A ${radius} ${radius} 0 ${sweep > 180 ? 1 : 0} 1 ${point(end)}`;
}

function lowest(windows) {
  return Math.min(
    ...windows
      .filter((window) => window.hasRemaining)
      .map((window) => window.remainingPercent),
  );
}

// Approximate widths only position the clock between the two independently
// rendered texts. SVG still uses the actual Yu Gothic glyph advances.
function resetWidth(text, size) {
  const units = { " ": 0.28, d: 0.59, h: 0.58, m: 0.88, "—": 1 };
  return [...text].reduce((width, char) => width + (units[char] ?? 0.556), 0) * size;
}

// The host supplies contract.icon as a PNG Data URL before calling this function.
// remaining updates claude-session; null represents an unknown remaining value.
export function svgDisplayData(sample, remaining) {
  const data = structuredClone(sample);
  const layout = structuredClone(svgLayout);
  const gauge = layout.gauges;
  const bars = layout.bars;
  const cellWidth = (layout.width - 2 * gauge.margin) / gauge.columns;
  const panelHeight = layout.height - gauge.panelTop - gauge.panelBottom;

  for (const [index, token] of data.tokens.entries()) {
    const top = bars.tokensTop + index * bars.tokensStep;
    token.barLabelY = top + 28;
    token.barValueY = top + 36 + token.tokensFontSize;
  }

  for (const contract of data.contracts) {
    contract.icon = contract.icon ?? null;
    for (const circle of contract.circles) {
      circle.double = circle.windows.length === 2;
      circle.showName = contract.circles.some((item) => item.name !== circle.name);
      circle.nameY = circle.double ? 186 : 190;
      for (const [index, window] of circle.windows.entries()) {
        if (window.id === "claude-session" && remaining !== undefined) {
          window.remainingPercent = remaining;
          window.tone = remaining < 25 ? "danger" : remaining <= 40 ? "warning" : "normal";
        }
        window.hasRemaining = window.remainingPercent !== null;
        window.percentText = window.hasRemaining ? `${Math.round(window.remainingPercent)}%` : "—";
        window.fillColor = colors[window.tone];
        const radius = index === 0 ? gauge.outerRadius : gauge.innerRadius;
        window.trackPath = arc(radius, 100);
        window.fillPath = window.hasRemaining ? arc(radius, window.remainingPercent) : "";
        window.percentageY = circle.double ? 92 + 31 * index : 108;
        window.resetY = gauge.resetBaseline + index * gauge.resetStep;
        const textWidth = resetWidth(window.resetText, gauge.resetFontSize);
        const labelWidth = Math.max(gauge.resetLabelWidth, resetWidth(window.durationLabel, gauge.resetFontSize));
        const rowWidth = labelWidth + gauge.resetLabelGap + gauge.clockSize + gauge.resetTextGap + textWidth;
        window.resetLabelX = (cellWidth - rowWidth) / 2;
        window.clockX = window.resetLabelX + labelWidth + gauge.resetLabelGap;
        window.resetTextX = window.clockX + gauge.clockSize + gauge.resetTextGap;
      }
    }
    contract.span = contract.circles.length;
    contract.windows = contract.circles.flatMap((circle) => circle.windows).slice(0, bars.maxRows);
  }

  let usedColumns = 0;
  data.gaugePanels = [];
  for (const contract of data.contracts.toSorted(
    (a, b) => lowest(a.circles.flatMap((circle) => circle.windows)) - lowest(b.circles.flatMap((circle) => circle.windows)),
  )) {
    if (usedColumns + contract.span > gauge.columns) continue;
    data.gaugePanels.push({
      ...contract,
      x: gauge.margin + usedColumns * cellWidth + gauge.panelGap / 2,
      y: gauge.panelTop,
      width: contract.span * cellWidth - gauge.panelGap,
      height: panelHeight,
      innerWidth: contract.span * cellWidth - gauge.panelGap - 2,
      innerHeight: panelHeight - 2,
      headingTextX: gauge.headingX + (contract.icon ? gauge.iconSize + gauge.iconGap : 0),
      circles: contract.circles.map((circle, index) => ({
        ...circle,
        x: index * cellWidth - gauge.panelGap / 2,
        gaugeX: (cellWidth - gauge.circleSize) / 2,
        gaugeScale: gauge.circleSize / 200,
      })),
    });
    usedColumns += contract.span;
  }

  data.barColumns = [];
  for (const contract of data.contracts.toSorted((a, b) => lowest(a.windows) - lowest(b.windows))) {
    let column = data.barColumns.find((item) =>
      item.contracts.length < bars.maxContracts &&
      item.contracts.reduce((count, existing) => count + existing.windows.length, 0) + contract.windows.length <= bars.maxRows,
    );
    if (!column && data.barColumns.length < bars.columns) {
      column = {
        x: bars.columnLeft + data.barColumns.length * (bars.columnWidth + bars.columnGap),
        y: bars.columnTop,
        contracts: [],
      };
      data.barColumns.push(column);
    }
    if (!column) continue;
    const y = column.contracts.reduce((height, existing) =>
      height + bars.headerHeight + existing.windows.length * bars.rowHeight + bars.stackGap, 0);
    column.contracts.push({
      ...contract,
      y,
      windows: contract.windows.map((window, index) => ({
        ...window,
        y: bars.headerHeight + index * bars.rowHeight,
        fillWidth: bars.columnWidth * Math.max(0, Math.min(100, window.remainingPercent)) / 100,
      })),
    });
  }
  return { ...data, width: layout.width, height: layout.height, colors, layout };
}
