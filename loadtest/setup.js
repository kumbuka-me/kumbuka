import http from "k6/http";
import { check, fail, sleep } from "k6";
import { baseURL } from "./lib/config.js";

export const options = { vus: 1, iterations: 1 };

function waitForHealth() {
  let health;
  for (let attempt = 0; attempt < 30; attempt += 1) {
    health = http.get(`${baseURL}/healthz`);
    if (health.status === 200) break;
    sleep(1);
  }
  if (!check(health, { "Kumbuka is healthy": (r) => r.status === 200 })) {
    fail("Kumbuka did not become healthy before setup.");
  }
}

export default function () {
  waitForHealth();

  // A fresh process registers /setup and renders it with 200. Once setup is
  // complete the route is intentionally absent; the authenticated-browser
  // fallback may answer 302 for this anonymous probe, while older/current
  // route shapes may answer 404. Both mean setup is already complete.
  const setupPage = http.get(`${baseURL}/setup`, {
    redirects: 0,
    responseCallback: http.expectedStatuses(200, 302, 404),
  });

  if (setupPage.status === 302 || setupPage.status === 404) {
    check(setupPage, {
      "initial setup already complete": (r) =>
        r.status === 302 || r.status === 404,
    });
    return;
  }

  if (setupPage.status !== 200) {
    fail(`Could not determine setup state (status ${setupPage.status}).`);
  }

  const setup = http.post(
    `${baseURL}/setup`,
    "username=loadtest-admin&email=loadtest-admin%40loadtest.invalid&display_name=Loadtest+Admin&password=loadtest-password&password_confirm=loadtest-password",
    {
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      redirects: 0,
      responseCallback: http.expectedStatuses(303),
    },
  );

  if (!check(setup, { "initial setup completed": (r) => r.status === 303 })) {
    fail(`Could not bootstrap the isolated database (status ${setup.status}).`);
  }
}
