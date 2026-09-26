export const baseURL = __ENV.BASE_URL || "http://127.0.0.1:8080";
export const pageCount = Number(__ENV.PAGE_COUNT || 48);

export function headers(user = "reader-1", admin = false) {
  return {
    "Content-Type": "application/json",
    "X-Forwarded-User": user,
    "X-Forwarded-Email": `${user}@loadtest.invalid`,
    "X-Forwarded-Name": user,
    ...(admin ? { "X-Forwarded-Groups": "loadtest-admins" } : {}),
  };
}

export function pageSlug(index) {
  return `docs/guides/guide-${index % pageCount}`;
}
