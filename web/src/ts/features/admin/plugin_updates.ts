import { route } from "../../core/route.ts";

type PluginUpdateProgress = {
  id: string;
  targetVersion: string;
  installedVersion: HTMLSpanElement;
  availableVersion: HTMLElement;
  progress: HTMLElement;
  spinner: HTMLElement;
  label: HTMLElement;
};

type PluginUpdateRowState = "updating" | "updated" | "failed" | "approval";

const pluginUpdateConcurrency = 4;
const boundPluginUpdateForms = new WeakSet<HTMLFormElement>();

// pluginUpdateForms returns the catalog update forms rendered below one DOM root.
function pluginUpdateForms(root: ParentNode = document): HTMLFormElement[] {
  return [...root.querySelectorAll<HTMLFormElement>("[data-plugin-update]")];
}

// isBulkPluginUpdate reports whether form is the Update all action.
function isBulkPluginUpdate(form: HTMLFormElement): boolean {
  try {
    return new URL(form.action, window.location.href).pathname.endsWith(
      "/admin/plugins/all/update",
    );
  } catch {
    return false;
  }
}

// pluginUpdateProblem extracts the server-rendered update error from a failed response.
function pluginUpdateProblem(result: Document): string {
  return (
    result.querySelector<HTMLElement>("[role='alert']")?.textContent?.trim() ||
    "Could not update the plugin from the Kumbuka catalog. Try again."
  );
}

// pluginUpdateProblemContainer returns the nearest surface where an update error belongs.
function pluginUpdateProblemContainer(
  form: HTMLFormElement,
): HTMLElement | null {
  return (
    form.closest<HTMLElement>(".plugin-update-section") ??
    form.closest<HTMLElement>(".settings-panel")
  );
}

// clearPluginUpdateProblem removes a previous catalog update error from this surface.
function clearPluginUpdateProblem(form: HTMLFormElement): void {
  pluginUpdateProblemContainer(form)
    ?.querySelector("[data-plugin-update-problem]")
    ?.remove();
}

// showPluginUpdateProblem keeps a failed update visible without replacing the current page.
function showPluginUpdateProblem(form: HTMLFormElement, message: string): void {
  const container = pluginUpdateProblemContainer(form);
  if (!container) return;

  let problem = container.querySelector<HTMLElement>(
    "[data-plugin-update-problem]",
  );
  if (!problem) {
    problem = document.createElement("p");
    problem.className = "settings-note";
    problem.dataset.pluginUpdateProblem = "";
    problem.setAttribute("role", "alert");

    const anchor =
      form.closest<HTMLElement>(".plugin-update-card") ??
      form.closest<HTMLElement>(".plugin-permission-approval") ??
      container.querySelector<HTMLElement>(".plugin-update-heading");
    if (anchor) anchor.insertAdjacentElement("afterend", problem);
    else container.prepend(problem);
  }

  problem.textContent = message;
}

// pluginUpdateControls returns the shared button feedback elements for one update form.
function pluginUpdateControls(form: HTMLFormElement): {
  submit: HTMLButtonElement;
  spinner: HTMLElement;
  label: HTMLElement;
} | null {
  const submit = form.querySelector<HTMLButtonElement>(
    "[data-plugin-update-submit]",
  );
  const spinner = form.querySelector<HTMLElement>(
    "[data-plugin-update-spinner]",
  );
  const label = form.querySelector<HTMLElement>("[data-plugin-update-label]");
  if (!submit || !spinner || !label) return null;

  return { submit, spinner, label };
}

// setPluginUpdatePending reflects one in-flight catalog update in its submit button.
function setPluginUpdatePending(
  form: HTMLFormElement,
  pending: boolean,
  text: string,
): void {
  const controls = pluginUpdateControls(form);
  if (!controls) return;

  if (pending) form.setAttribute("aria-busy", "true");
  else form.removeAttribute("aria-busy");

  controls.submit.disabled = pending;
  controls.spinner.hidden = !pending;
  controls.label.textContent = text;
}

// requestPluginUpdate performs one catalog update and returns its rendered response.
async function requestPluginUpdate(
  url: string,
  body?: BodyInit,
): Promise<{
  response: Response;
  result: Document;
}> {
  const response = await fetch(url, {
    method: "POST",
    credentials: "same-origin",
    headers: { Accept: "text/html" },
    body,
  });
  const result = new DOMParser().parseFromString(
    await response.text(),
    "text/html",
  );

  return { response, result };
}

// flushPendingPageRenders starts one deferred render rebuild after a bulk plugin update sequence.
async function flushPendingPageRenders(): Promise<void> {
  try {
    await fetch(route("/admin/pages/render-pending"), {
      method: "POST",
      credentials: "same-origin",
      headers: { Accept: "text/html" },
    });
  } catch {
    // A failed flush is safe: changed render plugins also change the render
    // fingerprint, so affected pages still rebuild lazily on their next read.
  }
}

// pluginPermissionApproval returns the permission challenge for one plugin from a rendered response.
function pluginPermissionApproval(
  result: Document,
  pluginID?: string,
): HTMLElement | null {
  for (const approval of result.querySelectorAll<HTMLElement>(
    "[data-plugin-permission-approval]",
  )) {
    if (!pluginID || approval.dataset.pluginId === pluginID) return approval;
  }
  return null;
}

// permissionApprovalCount returns both server-rendered and in-page approval states currently visible on the plugin list.
function permissionApprovalCount(): number {
  return (
    document.querySelectorAll("[data-plugin-permission-approval-required]")
      .length +
    document.querySelectorAll(
      '[data-plugin-update-progress][data-plugin-update-state="approval"]',
    ).length
  );
}

// pluginDetailDialog returns one plugin detail dialog without relying on a CSS-escaped plugin ID.
function pluginDetailDialog(
  root: ParentNode,
  pluginID: string,
): HTMLElement | null {
  for (const dialog of root.querySelectorAll<HTMLElement>(
    "[data-plugin-detail-dialog]",
  )) {
    if (dialog.dataset.pluginId === pluginID) return dialog;
  }
  return null;
}

// syncPermissionApprovalDetail copies the server-rendered permission review card into the current plugin dialog.
function syncPermissionApprovalDetail(
  pluginID: string,
  result: Document,
): boolean {
  const sourceDialog = pluginDetailDialog(result, pluginID);
  const targetDialog = pluginDetailDialog(document, pluginID);
  const source = sourceDialog?.querySelector<HTMLElement>(
    "[data-plugin-catalog-update]",
  );
  const target = targetDialog?.querySelector<HTMLElement>(
    "[data-plugin-catalog-update]",
  );
  if (!source || !target) return false;

  target.innerHTML = source.innerHTML;
  bindPluginUpdateForms(target);
  return true;
}

// updateSinglePlugin keeps the spinner visible for the complete blocking request and injects permission approval UI when required.
async function updateSinglePlugin(form: HTMLFormElement): Promise<void> {
  if (form.dataset.pluginUpdateRunning === "true") return;

  const controls = pluginUpdateControls(form);
  if (!controls) return;

  form.dataset.pluginUpdateRunning = "true";
  clearPluginUpdateProblem(form);

  const originalLabel = controls.label.textContent?.trim() || "Update plugin";
  const pendingLabel =
    form.dataset.pluginUpdatePending || "Downloading update…";
  let terminal = false;

  setPluginUpdatePending(form, true, pendingLabel);

  try {
    const { response, result } = await requestPluginUpdate(
      form.action,
      new FormData(form),
    );
    if (response.status === 409 && pluginPermissionApproval(result) !== null) {
      const pluginID =
        form.closest<HTMLElement>("[data-plugin-id]")?.dataset.pluginId;
      if (pluginID && syncPermissionApprovalDetail(pluginID, result)) {
        const rowUpdate = availablePluginUpdates().find(
          (update) => update.id === pluginID,
        );
        if (rowUpdate) {
          setPluginRowProgress(
            rowUpdate,
            "approval",
            "Permission approval required · Review permissions",
          );
        }
        terminal = true;
        return;
      }
    }

    if (!response.ok || !response.redirected) {
      showPluginUpdateProblem(form, pluginUpdateProblem(result));
      return;
    }

    terminal = true;
    window.location.assign(response.url);
  } catch {
    showPluginUpdateProblem(
      form,
      "Could not update the plugin. Check your connection and try again.",
    );
  } finally {
    delete form.dataset.pluginUpdateRunning;
    if (!terminal) setPluginUpdatePending(form, false, originalLabel);
  }
}

// availablePluginUpdates returns updateable rows in stable plugin-ID order. Rows already waiting for permission approval remain untouched until the administrator reviews them.
function availablePluginUpdates(): PluginUpdateProgress[] {
  const updates: PluginUpdateProgress[] = [];

  for (const row of document.querySelectorAll<HTMLTableRowElement>(
    "tr.plugin-row[data-plugin-detail-open]",
  )) {
    const id = row.dataset.pluginDetailOpen?.trim();
    if (!id) continue;
    if (row.querySelector("[data-plugin-permission-approval-required]"))
      continue;

    const availableVersion = row.querySelector<HTMLElement>(
      "[data-plugin-update-available]",
    );
    const installedVersion = row.querySelector<HTMLSpanElement>(
      "[data-plugin-installed-version]",
    );
    const nameCell = row.querySelector<HTMLTableCellElement>(
      "[data-plugin-name-cell]",
    );
    if (!availableVersion || !installedVersion || !nameCell) continue;

    const targetVersion =
      availableVersion.dataset.pluginUpdateVersion?.trim() ||
      (availableVersion.textContent || "").trim().replace(/\s+available$/i, "");
    if (!targetVersion) continue;

    let progress = nameCell.querySelector<HTMLElement>(
      "[data-plugin-update-progress]",
    );
    if (progress?.dataset.pluginUpdateState === "approval") continue;

    if (!progress) {
      progress = document.createElement("small");
      progress.className = "plugin-update-status";
      progress.dataset.pluginUpdateProgress = "";
      progress.setAttribute("role", "status");
      progress.setAttribute("aria-live", "polite");
      progress.hidden = true;

      const spinner = document.createElement("span");
      spinner.className = "plugin-update-spinner";
      spinner.dataset.pluginUpdateProgressSpinner = "";
      spinner.setAttribute("aria-hidden", "true");
      spinner.hidden = true;

      const label = document.createElement("span");
      label.dataset.pluginUpdateProgressLabel = "";

      progress.append(spinner, label);
      nameCell.append(progress);
    }

    const spinner = progress.querySelector<HTMLElement>(
      "[data-plugin-update-progress-spinner]",
    );
    const label = progress.querySelector<HTMLElement>(
      "[data-plugin-update-progress-label]",
    );
    if (!spinner || !label) continue;

    updates.push({
      id,
      targetVersion,
      installedVersion,
      availableVersion,
      progress,
      spinner,
      label,
    });
  }

  updates.sort((left, right) => left.id.localeCompare(right.id));
  return updates;
}

// setPluginRowProgress updates the visible state below the plugin name and description.
function setPluginRowProgress(
  update: PluginUpdateProgress,
  state: PluginUpdateRowState,
  message: string,
): void {
  update.progress.dataset.pluginUpdateState = state;
  update.progress.hidden = false;
  update.spinner.hidden = state !== "updating";
  update.label.textContent = message;
}

// markPluginUpdated updates the list row after a successful catalog upgrade.
function markPluginUpdated(update: PluginUpdateProgress): void {
  update.installedVersion.textContent = update.targetVersion;
  update.availableVersion.remove();
  setPluginRowProgress(update, "updated", `Updated to ${update.targetVersion}`);
}

// pluginUpdateURL returns the prefix-aware single-plugin catalog update route used by Update all.
function pluginUpdateURL(pluginID: string): string {
  return route(
    `/admin/plugins/${encodeURIComponent(pluginID)}/update?defer_render=1`,
  );
}

// updatePluginInBulk applies one independent catalog update. Its terminal state never throws into another worker.
async function updatePluginInBulk(
  update: PluginUpdateProgress,
): Promise<"updated" | "approval" | "failed"> {
  setPluginRowProgress(
    update,
    "updating",
    `Updating to ${update.targetVersion}…`,
  );

  let response: Response;
  let result: Document;
  try {
    ({ response, result } = await requestPluginUpdate(
      pluginUpdateURL(update.id),
    ));
  } catch {
    setPluginRowProgress(update, "failed", "Update failed · Retry available");
    return "failed";
  }

  if (
    response.status === 409 &&
    pluginPermissionApproval(result, update.id) !== null
  ) {
    syncPermissionApprovalDetail(update.id, result);
    setPluginRowProgress(
      update,
      "approval",
      "Permission approval required · Review permissions",
    );
    return "approval";
  }

  if (!response.ok || !response.redirected) {
    setPluginRowProgress(update, "failed", "Update failed · Retry available");
    return "failed";
  }

  markPluginUpdated(update);
  return "updated";
}

// updateAllPlugins runs up to four independent updates at once. Package downloads and validation overlap; the server-side plugin manager serializes the final lifecycle publication. A failed or approval-blocked plugin never stops another worker.
async function updateAllPlugins(form: HTMLFormElement): Promise<void> {
  if (form.dataset.pluginUpdateRunning === "true") return;

  const controls = pluginUpdateControls(form);
  if (!controls) return;

  const updates = availablePluginUpdates();
  if (updates.length === 0) {
    const approvals = permissionApprovalCount();
    setPluginUpdatePending(
      form,
      false,
      approvals > 0
        ? `Review approvals (${approvals})`
        : "All plugins are up to date",
    );
    controls.submit.disabled = true;
    return;
  }

  form.dataset.pluginUpdateRunning = "true";
  clearPluginUpdateProblem(form);
  controls.submit.disabled = true;
  form.setAttribute("aria-busy", "true");
  controls.spinner.hidden = false;
  controls.label.textContent = `Updating 0/${updates.length}…`;

  let nextIndex = 0;
  let finished = 0;
  let completed = 0;
  let approvals = 0;
  let failed = 0;

  const worker = async (): Promise<void> => {
    while (true) {
      const index = nextIndex++;
      if (index >= updates.length) return;

      const outcome = await updatePluginInBulk(updates[index]!);
      if (outcome === "updated") completed++;
      else if (outcome === "approval") approvals++;
      else failed++;

      finished++;
      controls.label.textContent = `Updating ${finished}/${updates.length}…`;
    }
  };

  try {
    const workerCount = Math.min(pluginUpdateConcurrency, updates.length);
    await Promise.all(Array.from({ length: workerCount }, () => worker()));
  } finally {
    if (completed > 0) await flushPendingPageRenders();

    delete form.dataset.pluginUpdateRunning;
    form.removeAttribute("aria-busy");
    controls.spinner.hidden = true;

    const pendingApprovals = permissionApprovalCount();
    if (failed > 0) {
      controls.label.textContent = `Retry failed (${failed})`;
      controls.submit.disabled = false;
    } else if (pendingApprovals > 0) {
      controls.label.textContent = `Review approvals (${pendingApprovals})`;
      controls.submit.disabled = true;
    } else {
      controls.label.textContent = `Updated all (${completed})`;
      controls.submit.disabled = true;
    }

    if (approvals > 0 || failed > 0) {
      const summary = [
        `${completed} updated`,
        `${approvals} approval required`,
        `${failed} failed`,
      ].join(" · ");
      const guidance =
        failed > 0
          ? "Failed updates can be retried; successful updates were kept."
          : "Open the marked plugins to review their permission changes.";
      showPluginUpdateProblem(form, `${summary}. ${guidance}`);
    }
  }
}

// bindPluginUpdateForms progressively enhances catalog update forms under one root exactly once.
function bindPluginUpdateForms(root: ParentNode = document): void {
  for (const form of pluginUpdateForms(root)) {
    if (boundPluginUpdateForms.has(form)) continue;
    boundPluginUpdateForms.add(form);

    form.addEventListener(
      "submit",
      (event: SubmitEvent) => {
        event.preventDefault();
        // plugins.ts contains the no-JavaScript navigation fallback handler.
        // Stop it here so this progressive enhancement owns the in-page request.
        event.stopImmediatePropagation();

        if (isBulkPluginUpdate(form)) {
          void updateAllPlugins(form);
          return;
        }

        void updateSinglePlugin(form);
      },
      { capture: true },
    );
  }
}

// initPluginUpdateProgress enables persistent single-update feedback and independent bulk progress.
export function initPluginUpdateProgress(): void {
  bindPluginUpdateForms();
}
