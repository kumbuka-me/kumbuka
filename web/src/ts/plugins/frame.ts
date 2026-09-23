// Core-owned harness. Plugin code runs only in this opaque, resource-restricted
// document. Outgoing messages report readiness, errors, bounded height, and
// trusted user interactions; core validates links and same-plugin command targets.
(() => {
  const commandIdentifier = /^[a-z0-9][a-z0-9._-]{0,127}$/;

  const isInitialPluginRenderEvent = (
    event: MessageEvent<unknown>,
    started: boolean,
  ): boolean =>
    !started &&
    event.source === parent &&
    typeof event.data === "object" &&
    event.data !== null;

  const isPluginRenderInput = (
    input: Record<string, unknown>,
  ): input is Record<string, unknown> & {
    type: "kumbuka-plugin-render";
    token: string;
    source: string;
    html?: string;
  } =>
    input.type === "kumbuka-plugin-render" &&
    typeof input.token === "string" &&
    typeof input.source === "string" &&
    input.source.length <= 1_000_000 &&
    (input.html === undefined ||
      (typeof input.html === "string" && input.html.length <= 1_000_000));

  const isTrustedPluginCommandClick = (
    event: MouseEvent,
    command: HTMLElement | null,
  ): command is HTMLElement =>
    event.isTrusted &&
    command !== null &&
    commandIdentifier.test(command.dataset.kumbukaCommandModule || "") &&
    commandIdentifier.test(command.dataset.kumbukaCommandAction || "");

  const isTrustedPluginCommandChange = (
    event: Event,
    target: EventTarget | null,
  ): target is HTMLSelectElement =>
    event.isTrusted &&
    target instanceof HTMLSelectElement &&
    commandIdentifier.test(target.dataset.kumbukaCommandModule || "") &&
    commandIdentifier.test(target.value);

  const isRenderablePluginModule = (
    module: unknown,
  ): module is {
    render: (root: HTMLElement, options: Record<string, string>) => unknown;
  } =>
    typeof module === "object" &&
    module !== null &&
    "render" in module &&
    typeof module.render === "function";

  let started = false;
  window.addEventListener("message", async (event: MessageEvent<unknown>) => {
    if (!isInitialPluginRenderEvent(event, started)) return;
    const input = event.data as Record<string, unknown>;
    if (!isPluginRenderInput(input)) return;
    // Keep the capability token private to the core harness. Browser-module
    // JavaScript is loaded only after this event has been consumed and cannot
    // observe or replay the token itself.
    event.stopImmediatePropagation();
    started = true;
    const send = (type: string, height = 0, width = 0) =>
      parent.postMessage({ type, token: input.token, height, width }, "*");
    const sendCommand = (module: string, action: string) =>
      parent.postMessage(
        {
          type: "kumbuka-plugin-command",
          token: input.token,
          module,
          action,
        },
        "*",
      );
    try {
      const url = document.body.dataset.pluginJavascript;
      const root = document.getElementById("plugin-root");
      if (!url || !root) throw new Error("Missing module");
      // Install the core-owned interaction bridge before loading plugin code.
      // Only trusted browser events can reach the command channel.
      root.addEventListener("click", (event) => {
        const target = event.target;
        const link =
          target instanceof Element
            ? target.closest<HTMLAnchorElement>("a[href]")
            : null;
        if (link) {
          event.preventDefault();
          if (event.isTrusted)
            parent.postMessage(
              {
                type: "kumbuka-plugin-link",
                token: input.token,
                href: link.href,
              },
              "*",
            );
          return;
        }

        const command =
          target instanceof Element
            ? target.closest<HTMLElement>(
                "[data-kumbuka-command-module][data-kumbuka-command-action]",
              )
            : null;
        if (isTrustedPluginCommandClick(event, command))
          sendCommand(
            command.dataset.kumbukaCommandModule || "",
            command.dataset.kumbukaCommandAction || "",
          );
      });
      root.addEventListener("change", (event) => {
        const target = event.target;
        if (!isTrustedPluginCommandChange(event, target)) return;

        sendCommand(target.dataset.kumbukaCommandModule || "", target.value);
      });
      await new Promise<void>((resolve, reject) => {
        const script = document.createElement("script");
        script.src = url;
        script.onload = () => resolve();
        script.onerror = () => reject(new Error("Module load failed"));
        document.head.append(script);
      });
      const module: unknown = (
        globalThis as unknown as { kumbukaPlugin?: unknown }
      ).kumbukaPlugin;
      if (!isRenderablePluginModule(module)) throw new Error("Invalid module");
      document.documentElement.style.colorScheme =
        input.theme === "dark" ? "dark" : "light";
      if (typeof input.colors === "object" && input.colors !== null) {
        for (const [name, value] of Object.entries(input.colors)) {
          if (
            /^[a-z-]{1,32}$/.test(name) &&
            typeof value === "string" &&
            CSS.supports("color", value)
          )
            document.documentElement.style.setProperty("--" + name, value);
        }
      }
      await module.render(root, {
        source: input.source,
        html: input.html || "",
        theme: input.theme === "dark" ? "dark" : "light",
      });
      const measure = () =>
        send(
          "kumbuka-plugin-ready",
          Math.min(
            10000,
            Math.max(
              24,
              Math.ceil(
                Math.max(
                  root.getBoundingClientRect().height,
                  root.scrollHeight,
                ),
              ),
            ),
          ),
          Math.min(
            480,
            Math.max(
              72,
              Math.ceil(
                Math.max(root.getBoundingClientRect().width, root.scrollWidth),
              ),
            ),
          ),
        );
      measure();
      new ResizeObserver(measure).observe(root);
      let queued = false;
      new MutationObserver(() => {
        if (queued) return;
        queued = true;
        requestAnimationFrame(() => {
          queued = false;
          measure();
        });
      }).observe(root, {
        subtree: true,
        childList: true,
        attributes: true,
        characterData: true,
      });
    } catch {
      send("kumbuka-plugin-error");
    }
  });
  parent.postMessage({ type: "kumbuka-plugin-listening" }, "*");
})();
