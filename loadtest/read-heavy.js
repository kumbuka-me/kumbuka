import http from "k6/http";
import { check, sleep } from "k6";
import { baseURL, headers, pageSlug } from "./lib/config.js";

export const options = {
  scenarios: {
    readers: {
      executor: "ramping-vus",
      startVUs: 0,
      stages: [
        { duration: "30s", target: 10 },
        { duration: "2m", target: 10 },
        { duration: "30s", target: 0 },
      ],
    },
  },
  thresholds: {
    http_req_failed: ["rate<0.01"],
    http_req_duration: ["p(95)<500", "p(99)<1000"],
  },
};

export default function () {
  const params = { headers: headers(`reader-${__VU}`) };
  const slug = pageSlug(__ITER + __VU);
  const responses = http.batch([
    ["GET", `${baseURL}/api/pages/${slug}`, null, params],
    ["GET", `${baseURL}/api/pages`, null, params],
    ["GET", `${baseURL}/api/search?q=seeded`, null, params],
  ]);
  check(responses[0], { "page read succeeds": (r) => r.status === 200 });
  check(responses[1], { "page list succeeds": (r) => r.status === 200 });
  check(responses[2], { "search succeeds": (r) => r.status === 200 });
  sleep(1);
}
