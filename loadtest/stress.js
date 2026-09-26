import http from "k6/http";
import { check } from "k6";
import exec from "k6/execution";
import { baseURL, headers, pageSlug } from "./lib/config.js";

const stressHeaders = headers("stress-reader");

export const options = {
  discardResponseBodies: true,
  scenarios: {
    readers: {
      executor: "ramping-arrival-rate",
      startRate: 2000,
      timeUnit: "1s",
      preAllocatedVUs: 250,
      maxVUs: 3500,
      stages: [
        { duration: "15s", target: 2400 },
        { duration: "30s", target: 2400 },
        { duration: "10s", target: 2500 },
        { duration: "30s", target: 2500 },
        { duration: "10s", target: 2600 },
        { duration: "30s", target: 2600 },
        { duration: "10s", target: 2700 },
        { duration: "30s", target: 2700 },
        { duration: "10s", target: 2800 },
        { duration: "30s", target: 2800 },
        { duration: "10s", target: 2900 },
        { duration: "30s", target: 2900 },
        { duration: "10s", target: 3000 },
        { duration: "30s", target: 3000 },
        { duration: "15s", target: 0 },
      ],
    },
  },
  thresholds: {
    http_req_failed: ["rate<0.02"],
    http_req_duration: ["p(95)<1000", "p(99)<2000"],
    dropped_iterations: ["count==0"],
  },
};

function params(name) {
  return {
    headers: stressHeaders,
    tags: { name },
  };
}

export default function () {
  const iteration = exec.scenario.iterationInTest;
  const workload = iteration % 10;

  if (workload < 7) {
    const response = http.get(
      `${baseURL}/api/pages/${pageSlug(iteration)}`,
      params("GET /api/pages/:slug"),
    );
    check(response, { "stress page read succeeds": (r) => r.status === 200 });
    return;
  }

  if (workload < 9) {
    const response = http.get(
      `${baseURL}/api/pages`,
      params("GET /api/pages"),
    );
    check(response, { "stress page list succeeds": (r) => r.status === 200 });
    return;
  }

  const response = http.get(
    `${baseURL}/api/search?q=seeded`,
    params("GET /api/search"),
  );
  check(response, { "stress search succeeds": (r) => r.status === 200 });
}
