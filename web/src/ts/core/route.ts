// The body carries the same normalized deployment prefix used by Go templates.
export function routePrefix(): string {
  return typeof document === "undefined"
    ? ""
    : document.body?.dataset.routePrefix || "";
}

// Call with prefix-free local paths. Preserve external/relative references; never let a local path escape the mount.
export function route(target: string, prefix = routePrefix()): string {
  if (!target.startsWith("/")) return target;
  try {
    const path = decodeURIComponent(target.split(/[?#]/, 1)[0]!);
    if (
      path.startsWith("//") ||
      /[\\\r\n]/.test(path) ||
      path.split("/").some((part) => part === "." || part === "..")
    )
      return prefix + "/";
    return prefix + target;
  } catch {
    return prefix + "/";
  }
}

// Returns the application-local canonical URL for a persisted page.
export function pageURL(id: number, slug: string): string {
  return `/p/${id}/${slug.replace(/^\/+|\/+$/g, "")}`;
}
