import http from "k6/http";
import { check } from "k6";
import { baseURL, headers } from "./lib/config.js";

const stressHeaders = headers("search-stress-reader");

export const options = {
  discardResponseBodies: true,
  scenarios: {
    searchers: {
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
    http_req_duration: ["p(95)<1000", "p(99)<2000"],
    dropped_iterations: ["count==0"],
  },
};

export default function () {
  const response = http.get(`${baseURL}/api/search?q=seeded`, {
    headers: stressHeaders,
    tags: { name: "GET /api/search" },
  });
  check(response, { "search stress succeeds": (r) => r.status === 200 });
}
