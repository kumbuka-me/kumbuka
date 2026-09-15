// Core-owned harness. Plugin code runs only in this opaque, resource-restricted
// document. Outgoing messages report readiness, errors, bounded height, and
// user clicks; core validates link destinations against the original fallback.
(() => {
  let started = false;
  window.addEventListener("message", async (event: MessageEvent<unknown>) => {
    if (
      started ||
      event.source !== parent ||
      typeof event.data !== "object" ||
      event.data === null
    )
      return;
    const input = event.data as Record<string, unknown>;
    if (
      input.type !== "kumbuka-plugin-render" ||
      typeof input.token !== "string" ||
      typeof input.source !== "string" ||
      input.source.length > 1_000_000 ||
      (input.html !== undefined &&
        (typeof input.html !== "string" || input.html.length > 1_000_000))
    )
      return;
    started = true;
    const send = (type: string, height = 0) =>
      parent.postMessage({ type, token: input.token, height }, "*");
    try {
      const url = document.body.dataset.pluginJavascript;
      const root = document.getElementById("plugin-root");
      if (!url || !root) throw new Error("Missing module");
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
      if (
        typeof module !== "object" ||
        module === null ||
        !("render" in module) ||
        typeof module.render !== "function"
      )
        throw new Error("Invalid module");
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
      root.addEventListener("click", (event) => {
        const target = event.target;
        const link =
          target instanceof Element
            ? target.closest<HTMLAnchorElement>("a[href]")
            : null;
        if (!link) return;
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
      });
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
