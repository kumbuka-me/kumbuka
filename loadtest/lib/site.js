const projectReadme = open("/source/README.md");
const pluginReadme = open("/source/plugin-README.md");

function guide(index, imageURL) {
  const previous =
    index === 0
      ? "/pages/docs/overview"
      : `/pages/docs/guides/guide-${index - 1}`;
  const next =
    index === 47
      ? "/pages/docs/plugin-development"
      : `/pages/docs/guides/guide-${index + 1}`;
  const rows = Array.from(
    { length: 12 },
    (_, row) => `| ${row + 1} | service-${index}-${row} | healthy |`,
  ).join("\n");
  const code = Array.from(
    { length: 16 },
    (_, line) => `  step_${line}: verify component ${index}`,
  ).join("\n");
  const image =
    index % 8 === 0 && imageURL
      ? `\n![Kumbuka architecture fixture](${imageURL}){width=320px}\n`
      : "";
  return {
    slug: `docs/guides/guide-${index}`,
    title: `Operations guide ${index + 1}`,
    tags: [
      "documentation",
      "operations",
      index % 2 === 0 ? "platform" : "engineering",
    ],
    markdown: `# Operations guide ${index + 1}\n\nThis deterministic guide is part of the Kumbuka load-test documentation site.\n\n## Checklist\n\n- [x] Confirm the previous run completed.\n- [ ] Review the linked configuration.\n- [ ] Record the deployment result.\n\n## Service matrix\n\n| Order | Service | State |\n| --- | --- | --- |\n${rows}\n\n## Procedure\n\n\`\`\`yaml\nrunbook:\n${code}\n\`\`\`${image}\nRead [the previous section](${previous}) or [continue](${next}).\n`,
  };
}

export function sitePages(imageURL = "") {
  const pages = [
    {
      slug: "docs/overview",
      title: "Kumbuka overview",
      tags: ["documentation", "overview"],
      markdown: projectReadme,
    },
    {
      slug: "docs/plugin-development",
      title: "Kumbuka plugin development",
      tags: ["documentation", "plugins"],
      markdown: pluginReadme,
    },
    {
      slug: "docs/architecture",
      title: "Kumbuka architecture",
      tags: ["documentation", "architecture"],
      markdown: `# Kumbuka architecture\n\nStart with the [overview](/pages/docs/overview), then follow the generated operations guides.\n${imageURL ? `\n![Architecture fixture](${imageURL}){width=480px}\n` : ""}`,
    },
  ];
  for (let index = 0; index < 48; index += 1)
    pages.push(guide(index, imageURL));
  return pages;
}
