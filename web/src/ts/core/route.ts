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
    if (unsafeLocalPath(path)) return prefix + "/";
    return prefix + target;
  } catch {
    return prefix + "/";
  }
}

// Rejects decoded path segments that browsers can reinterpret outside the mount.
function unsafeLocalPath(path: string): boolean {
  return (
    path.startsWith("//") ||
    /[\\\t\r\n]/.test(path) ||
    path.split("/").some((part) => part === "." || part === "..")
  );
}

// Returns the application-local canonical URL for a persisted page.
export function pageURL(id: number, slug: string): string {
  const path = slug.trim().replace(/^\/+|\/+$/g, "");
  return path ? `/p/${id}/${path}` : `/p/${id}`;
}
