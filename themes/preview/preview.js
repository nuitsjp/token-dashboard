const selector = document.querySelector("#window");
const remaining = document.querySelector("#remaining");
const state = document.querySelector("#state");
const frames = [];
const fixtures = new Map();

function update() {
  document.querySelector("#value").textContent = `${remaining.value}%`;
  for (const { frame, theme } of frames) {
    const params = new URLSearchParams({
      theme: theme.id,
      window: selector.value,
      remaining: remaining.value,
      state: state.value,
    });
    frame.src = `/render?${params}`;
  }
}

try {
  const [themesResponse, sampleResponse] = await Promise.all([
    fetch("/themes"),
    fetch("/preview/sample-data.json"),
  ]);
  if (!themesResponse.ok || !sampleResponse.ok)
    throw new Error("テーマまたはサンプルデータを読み込めませんでした。");
  const [themes, sample] = await Promise.all([
    themesResponse.json(),
    sampleResponse.json(),
  ]);
  for (const contract of sample.contracts) {
    for (const circle of contract.circles) {
      for (const window of circle.windows) {
        fixtures.set(window.id, window);
        selector.add(
          new Option(`${contract.provider} / ${window.label}`, window.id),
        );
      }
    }
  }
  for (const theme of themes) {
    const section = document.createElement("section");
    const heading = document.createElement("h2");
    heading.textContent = `${theme.name} · ${theme.width} × ${theme.height}`;
    const preview = document.createElement("div");
    preview.className = "preview";
    const frame = document.createElement("iframe");
    frame.title = theme.name;
    frame.dataset.theme = theme.id;
    frame.sandbox = "allow-same-origin";
    frame.style.width = `${theme.width}px`;
    frame.style.height = `${theme.height}px`;
    preview.style.aspectRatio = `${theme.width} / ${theme.height}`;
    preview.append(frame);
    section.append(heading, preview);
    document.querySelector("#previews").append(section);
    new ResizeObserver(() => {
      frame.style.transform = `scale(${preview.clientWidth / theme.width})`;
    }).observe(preview);
    frames.push({ frame, theme });
  }
  selector.addEventListener("change", () => {
    const selected = fixtures.get(selector.value);
    state.value = selected.remainingPercent === null ? "unknown" : "ready";
    remaining.value =
      selected.remainingPercent === null ? 0 : selected.remainingPercent;
    update();
  });
  remaining.addEventListener("input", update);
  state.addEventListener("change", update);
  update();
} catch (error) {
  document.querySelector("#error").textContent = error.message;
}
