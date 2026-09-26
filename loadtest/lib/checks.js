import { check } from "k6";

export function isOK(response, name) {
  return check(response, {
    [`${name}: successful response`]: (r) => r.status >= 200 && r.status < 300,
  });
}
