import http from "k6/http";
import encoding from "k6/encoding";
import { check, fail, sleep } from "k6";
import { baseURL, headers } from "./lib/config.js";
import { sitePages } from "./lib/site.js";

export const options = { vus: 1, iterations: 1 };

function request(method, path, body) {
  return http.request(method, `${baseURL}${path}`, body, {
    headers: headers("seed", true),
    responseCallback: http.expectedStatuses(201, 409),
  });
}

function architectureImage() {
  const existing = http.get(
    `${baseURL}/api/images?q=loadtest-architecture&scope=mine`,
    { headers: headers("seed", true) },
  );
  if (existing.status === 200 && existing.json().length > 0)
    return existing.json()[0].url;
  const png = encoding.b64decode(
    "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mP8/x8AAusB9WlO2wAAAABJRU5ErkJggg==",
    "std",
  );
  const mediaHeaders = headers("seed", true);
  delete mediaHeaders["Content-Type"];
  const uploaded = http.post(
    `${baseURL}/api/images`,
    { file: http.file(png, "loadtest-architecture.png", "image/png") },
    { headers: mediaHeaders, responseCallback: http.expectedStatuses(201) },
  );
  if (
    !check(uploaded, { "architecture image uploaded": (r) => r.status === 201 })
  )
    fail(`Could not upload the load-test image (status ${uploaded.status}).`);
  return uploaded.json("url");
}

export default function () {
  let health;
  for (let attempt = 0; attempt < 30; attempt += 1) {
    health = http.get(`${baseURL}/healthz`);
    if (health.status === 200) break;
    sleep(1);
  }
  if (!check(health, { "Kumbuka is healthy": (r) => r.status === 200 })) {
    fail("Kumbuka did not become healthy before seed data was created.");
  }

  // A fresh database requires its first administrator before the trusted-proxy
  // override can authenticate workload users. Repeated seeds get 404 because
  // setup is intentionally a one-time operation.
  const setup = http.post(
    `${baseURL}/setup`,
    "username=loadtest-admin&email=loadtest-admin%40loadtest.invalid&display_name=Loadtest+Admin&password=loadtest-password&password_confirm=loadtest-password",
    {
      headers: { "Content-Type": "application/x-www-form-urlencoded" },
      redirects: 0,
      responseCallback: http.expectedStatuses(303, 404),
    },
  );
  if (
    !check(setup, {
      "initial setup completed or was already complete": (r) =>
        r.status === 303 || r.status === 404,
    })
  ) {
    fail(`Could not bootstrap the isolated database (status ${setup.status}).`);
  }

  const imageURL = architectureImage();
  for (const page of sitePages(imageURL)) {
    const response = request(
      "POST",
      "/api/pages",
      JSON.stringify({
        slug: page.slug,
        title: page.title,
        markdown_content: page.markdown,
        message: "Seed load-test data",
        status: "verified",
        tags: [...page.tags, "loadtest"],
      }),
    );
    check(response, {
      [`seed ${page.slug}: created or already present`]: (r) =>
        r.status === 201 || r.status === 409,
    });
  }
}
