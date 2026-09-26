import http from "k6/http";
import { check } from "k6";
import exec from "k6/execution";
import { baseURL, headers, pageSlug } from "./lib/config.js";

const stressHeaders = headers("render-stress-reader");

export const options = {
  discardResponseBodies: true,
  scenarios: {
    renderers: {
      executor: "ramping-arrival-rate",
      startRate: 50,
      timeUnit: "1s",
      preAllocatedVUs: 100,
      maxVUs: 2000,
      stages: [
        { duration: "30s", target: 100 },
        { duration: "1m", target: 250 },
        { duration: "1m", target: 500 },
        { duration: "1m", target: 1000 },
        { duration: "1m", target: 2000 },
        { duration: "1m", target: 2000 },
        { duration: "30s", target: 0 },
      ],
    },
  },
  thresholds: {
    http_req_failed: ["rate<0.02"],
    http_req_duration: ["p(95)<1500", "p(99)<3000"],
    dropped_iterations: ["count==0"],
  },
};

export default function () {
  const slug = pageSlug(exec.scenario.iterationInTest);
  const response = http.get(`${baseURL}/pages/${slug}`, {
    headers: stressHeaders,
    tags: { name: "GET /pages/:slug" },
  });
  check(response, { "render stress succeeds": (r) => r.status === 200 });
}
