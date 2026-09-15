// Classic, parser-blocking script: apply the server-selected palette before
// the body can paint, without waiting for the application's module graph.
(() => {
  const source = document.getElementById("kumbuka-themes");
  if (!source) return;

  const catalog = JSON.parse(source.textContent ?? "[]") as {
    title: string;
    color_scheme: string;
    colors: Record<string, string>;
  }[];
  const root = document.documentElement;
  const title = (root.dataset.theme || "Light").toLocaleLowerCase();
  const theme =
    catalog.find(
      (candidate) => candidate.title.toLocaleLowerCase() === title,
    ) ?? catalog[0];
  if (!theme) return;

  for (const [key, value] of Object.entries(theme.colors)) {
    root.style.setProperty(`--${key.replaceAll("_", "-")}`, value);
  }
  root.style.colorScheme = theme.color_scheme;
  root.dataset.theme = theme.title;
})();
