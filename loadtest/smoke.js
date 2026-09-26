import http from "k6/http";
import { check } from "k6";
import { baseURL, headers, pageSlug } from "./lib/config.js";

export const options = {
  vus: 1,
  iterations: 1,
  thresholds: {
    http_req_failed: ["rate==0"],
    http_req_duration: ["p(95)<1000"],
  },
};

export default function () {
  const params = { headers: headers("smoke-reader") };
  const responses = http.batch([
    ["GET", `${baseURL}/api/pages`, null, params],
    ["GET", `${baseURL}/api/pages/${pageSlug(0)}`, null, params],
    ["GET", `${baseURL}/api/search?q=seeded`, null, params],
  ]);

  check(responses[0], {
    "page list is available": (r) => r.status === 200 && r.json().length > 0,
  });
  check(responses[1], {
    "seeded page is available": (r) =>
      r.status === 200 && r.json("slug") === pageSlug(0),
  });
  check(responses[2], { "search is available": (r) => r.status === 200 });
}
