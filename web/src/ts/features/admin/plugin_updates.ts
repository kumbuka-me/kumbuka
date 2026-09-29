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

type PluginUpdateRowState =
  "queued" | "updating" | "updated" | "failed" | "approval";

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

// staticPermissionApprovalCount returns server-rendered approval markers already present on the plugin list.
function staticPermissionApprovalCount(): number {
  return document.querySelectorAll("[data-plugin-permission-approval-required]")
    .length;
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
            "Permission approval required",
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

// availablePluginUpdates returns updateable rows in stable plugin-ID order.
function availablePluginUpdates(): PluginUpdateProgress[] {
  const updates: PluginUpdateProgress[] = [];

  for (const row of document.querySelectorAll<HTMLTableRowElement>(
    "tr.plugin-row[data-plugin-detail-open]",
  )) {
    const id = row.dataset.pluginDetailOpen?.trim();
    if (!id) continue;

    const availableVersion = row.querySelector<HTMLElement>(
      ".plugin-update-version:not([data-plugin-update-progress]):not([data-plugin-permission-approval-required])",
    );
    const versionCell = availableVersion?.closest<HTMLTableCellElement>("td");
    const installedVersion = versionCell?.querySelector<HTMLSpanElement>(
      "span:not(.plugin-update-spinner)",
    );
    if (!availableVersion || !versionCell || !installedVersion) continue;

    const targetVersion = (availableVersion.textContent || "")
      .trim()
      .replace(/\s+available$/i, "");
    if (!targetVersion) continue;

    let progress = versionCell.querySelector<HTMLElement>(
      "[data-plugin-update-progress]",
    );
    if (!progress) {
      progress = document.createElement("small");
      progress.className = "plugin-update-version";
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

      progress.append(spinner, document.createTextNode(" "), label);
      versionCell.append(progress);
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

// setPluginRowProgress updates the visible state for one plugin in a bulk update.
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

// updateAllPlugins runs every update sequentially so one plugin failure cannot prevent later plugins from being attempted and each row can expose real progress.
async function updateAllPlugins(form: HTMLFormElement): Promise<void> {
  if (form.dataset.pluginUpdateRunning === "true") return;

  const controls = pluginUpdateControls(form);
  if (!controls) return;

  const updates = availablePluginUpdates();
  if (updates.length === 0) {
    const approvals = staticPermissionApprovalCount();
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

  for (const update of updates) {
    setPluginRowProgress(
      update,
      "queued",
      `Queued for ${update.targetVersion}`,
    );
  }

  controls.submit.disabled = true;
  form.setAttribute("aria-busy", "true");
  controls.spinner.hidden = false;

  let completed = 0;
  let approvals = 0;
  let failed = 0;

  try {
    for (let index = 0; index < updates.length; index++) {
      const update = updates[index]!;
      controls.label.textContent = `Updating ${index + 1}/${updates.length}…`;
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
        failed++;
        setPluginRowProgress(update, "failed", "Update failed");
        continue;
      }

      if (
        response.status === 409 &&
        pluginPermissionApproval(result, update.id) !== null
      ) {
        approvals++;
        syncPermissionApprovalDetail(update.id, result);
        setPluginRowProgress(
          update,
          "approval",
          "Permission approval required",
        );
        continue;
      }

      if (!response.ok || !response.redirected) {
        failed++;
        setPluginRowProgress(update, "failed", "Update failed");
        continue;
      }

      markPluginUpdated(update);
      completed++;
    }
  } finally {
    if (completed > 0) await flushPendingPageRenders();

    delete form.dataset.pluginUpdateRunning;
    form.removeAttribute("aria-busy");
    controls.spinner.hidden = true;

    const retryableRemaining = availablePluginUpdates().length;
    const staticApprovals = staticPermissionApprovalCount();
    const remaining = retryableRemaining + staticApprovals;
    if (remaining === 0) {
      controls.label.textContent = `Updated all (${completed})`;
      controls.submit.disabled = true;
    } else if (
      failed === 0 &&
      retryableRemaining === approvals &&
      remaining === approvals + staticApprovals
    ) {
      controls.label.textContent = `Review approvals (${remaining})`;
      controls.submit.disabled = true;
    } else {
      controls.label.textContent = `Retry remaining (${remaining})`;
      controls.submit.disabled = false;
    }

    const problems: string[] = [];
    if (approvals > 0) {
      problems.push(
        `${approvals} update(s) need permission approval. Open the marked plugins to review the permission changes.`,
      );
    }
    if (failed > 0) {
      problems.push(
        `${failed} update(s) failed. Other plugins were still attempted and successful updates were kept.`,
      );
    }
    if (problems.length > 0) showPluginUpdateProblem(form, problems.join(" "));
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
