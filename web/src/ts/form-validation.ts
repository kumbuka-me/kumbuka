// Reusable client-side validation for server-backed Kumbuka forms.

import {
  problemFieldLabel,
  showProblemDialog,
  type ProblemDialogDetail,
} from "./core/dialogs.ts";
import {
  followedRedirectURL,
  parseProblemPayload,
  type ProblemPayload,
} from "./core/http.ts";
import { localPasswordProblem } from "./core/password.ts";

type FormControl = HTMLInputElement | HTMLSelectElement | HTMLTextAreaElement;

const formSelector = "form[data-validate-form]";
const errorClass = "field-validation-error";
const invalidClass = "is-invalid";
const shakeClass = "validation-shake";

function isFormControl(control: unknown): control is FormControl {
  return (
    control instanceof HTMLInputElement ||
    control instanceof HTMLSelectElement ||
    control instanceof HTMLTextAreaElement
  );
}

function controlFor(form: HTMLFormElement, name: string): FormControl | null {
  const proxy = [
    ...form.querySelectorAll<FormControl>("[data-error-field]"),
  ].find((control) => control.dataset.errorField === name);
  if (proxy) return proxy;

  const control = form.elements.namedItem(name);
  if (isFormControl(control)) return control;

  return null;
}

function errorID(control: FormControl): string {
  const formName = control.form?.dataset.validationName || "form";
  return `validation-${formName}-${control.name}`.replace(
    /[^a-zA-Z0-9_-]/g,
    "-",
  );
}

function errorContainer(control: FormControl): HTMLElement {
  return control.closest("label") || control.parentElement || control;
}

function setDescribedBy(control: FormControl, id: string): void {
  const ids = new Set(
    (control.getAttribute("aria-describedby") || "")
      .split(/\s+/)
      .filter(Boolean),
  );

  ids.add(id);
  control.setAttribute("aria-describedby", [...ids].join(" "));
}

function clearDescribedBy(control: FormControl, id: string): void {
  const ids = (control.getAttribute("aria-describedby") || "")
    .split(/\s+/)
    .filter((value) => value && value !== id);
  if (ids.length > 0) control.setAttribute("aria-describedby", ids.join(" "));
  else control.removeAttribute("aria-describedby");
}

function shake(control: FormControl): void {
  control.classList.remove(shakeClass);
  void control.offsetWidth;
  control.classList.add(shakeClass);
}

function showFieldError(
  control: FormControl,
  message: string,
  animate = true,
): void {
  const id = errorID(control);
  let error = document.getElementById(id);

  if (!error) {
    error = document.createElement("small");
    error.id = id;
    error.className = errorClass;
    error.setAttribute("role", "alert");
    errorContainer(control).append(error);
  }

  error.textContent = message;
  control.classList.add(invalidClass);
  control.setAttribute("aria-invalid", "true");
  setDescribedBy(control, id);

  if (animate) shake(control);
}

function clearFieldError(control: FormControl): void {
  const id = errorID(control);

  document.getElementById(id)?.remove();
  control.classList.remove(invalidClass, shakeClass);
  control.removeAttribute("aria-invalid");
  clearDescribedBy(control, id);
}

function localMessage(control: FormControl): string {
  if (control.validity.valueMissing) {
    return control.dataset.errorRequired || "This field is required.";
  }
  if (control.hasAttribute("data-validate-password") && control.value !== "") {
    const problem = localPasswordProblem(control.value);
    if (problem) return problem;
  }
  if (control.validity.typeMismatch) {
    return control.dataset.errorType || control.validationMessage;
  }
  if (control.validity.tooShort) {
    return control.dataset.errorMinlength || control.validationMessage;
  }
  if (!control.validity.valid) {
    return control.validationMessage;
  }

  const match = control.dataset.validateMatch;

  if (match) {
    const expected = controlFor(control.form!, match);
    if (expected && expected.value !== "" && control.value === "") {
      return control.dataset.errorRequired || "Confirm the value.";
    }
    if (expected && control.value !== expected.value) {
      return control.dataset.errorMatch || "Values do not match.";
    }
  }

  return "";
}

function validateControl(control: FormControl, animate = true): boolean {
  clearFieldError(control);
  if (control.disabled || !control.willValidate) return true;

  const message = localMessage(control);
  if (!message) return true;

  showFieldError(control, message, animate);
  return false;
}

function revalidateDependents(form: HTMLFormElement, name: string): void {
  for (const dependent of form.querySelectorAll<FormControl>(
    `[data-validate-match="${CSS.escape(name)}"]`,
  )) {
    if (dependent.classList.contains(invalidClass)) {
      validateControl(dependent, false);
    }
  }
}

function clearForm(form: HTMLFormElement): void {
  clearFormMessage(form);
  for (const control of form.elements) {
    if (isFormControl(control)) clearFieldError(control);
  }
}

function validateForm(form: HTMLFormElement): boolean {
  let firstInvalid: FormControl | null = null;

  for (const control of form.elements) {
    if (!isFormControl(control)) continue;
    if (!validateControl(control)) firstInvalid ||= control;
  }

  firstInvalid?.focus();
  return firstInvalid === null;
}

function clearFormMessage(form: HTMLFormElement): void {
  form.querySelector<HTMLElement>("[data-validation-form-error]")?.remove();
}

function showFormMessage(form: HTMLFormElement, message: string): void {
  clearFormMessage(form);

  const error = document.createElement("div");

  error.className = "form-validation-error";
  error.dataset.validationFormError = "true";
  error.setAttribute("role", "alert");
  error.textContent = message;
  form.prepend(error);
}

function controlLabel(control: FormControl, name: string): string {
  const explicit = control.dataset.errorLabel?.trim();
  if (explicit) return explicit;

  const labelled = control.labels?.item(0);
  if (labelled) {
    const clone = labelled.cloneNode(true) as HTMLElement;
    clone
      .querySelectorAll("small, .hint, .field-validation-error")
      .forEach((node) => node.remove());
    const text = clone.textContent?.replace(/\s+/g, " ").trim();
    if (text) return text;
  }

  return problemFieldLabel(name);
}

function showServerProblems(
  form: HTMLFormElement,
  problem: ProblemPayload,
): { firstInvalid: FormControl | null; details: ProblemDialogDetail[] } {
  let firstInvalid: FormControl | null = null;
  const details: ProblemDialogDetail[] = [];

  for (const [name, message] of Object.entries(problem.problems || {})) {
    if (!message.trim()) continue;

    const control = controlFor(form, name);
    details.push({
      field: name,
      label: control ? controlLabel(control, name) : problemFieldLabel(name),
      message,
    });

    if (!control) continue;

    clearFieldError(control);
    showFieldError(control, message);
    firstInvalid ||= control;
  }

  return { firstInvalid, details };
}

function formBody(form: HTMLFormElement): URLSearchParams {
  const body = new URLSearchParams();

  for (const [name, value] of new FormData(form)) {
    if (typeof value === "string") body.append(name, value);
  }

  return body;
}

async function submitForm(form: HTMLFormElement): Promise<void> {
  clearFormMessage(form);

  const submitters = [
    ...form.querySelectorAll<HTMLButtonElement | HTMLInputElement>(
      'button[type="submit"], input[type="submit"]',
    ),
  ];

  submitters.forEach((button) => (button.disabled = true));

  try {
    const response = await fetch(form.action || window.location.href, {
      method: form.method || "POST",
      body: formBody(form),
      credentials: "same-origin",
      headers: { Accept: "application/json" },
    });

    const redirectURL = followedRedirectURL(response);
    if (redirectURL) {
      form.dispatchEvent(new CustomEvent("kumbuka:form-submit-success"));
      window.location.assign(redirectURL);
      return;
    }

    if (response.ok) {
      form.dispatchEvent(new CustomEvent("kumbuka:form-submit-success"));
      window.location.reload();
      return;
    }

    const contentType = response.headers.get("Content-Type") || "";
    const problem = contentType.includes("application/json")
      ? parseProblemPayload(await response.json())
      : { error: "The form could not be submitted." };
    const { firstInvalid, details } = showServerProblems(form, problem);
    const shown = await showProblemDialog(problem, {
      title: form.dataset.errorTitle || "Could not submit form",
      details,
    });

    if (!shown && !firstInvalid) {
      showFormMessage(
        form,
        problem.error || "The form could not be submitted.",
      );
    }

    firstInvalid?.scrollIntoView({ behavior: "smooth", block: "center" });
    firstInvalid?.focus({ preventScroll: true });
  } catch {
    const problem = {
      error:
        "The form could not be submitted. Check your connection and try again.",
    };
    const shown = await showProblemDialog(problem, {
      title: form.dataset.errorTitle || "Could not submit form",
    });
    if (!shown) showFormMessage(form, problem.error);
  } finally {
    submitters.forEach((button) => (button.disabled = false));
  }
}

function initForm(form: HTMLFormElement, index: number): void {
  form.noValidate = true;
  form.dataset.validationName ||= String(index + 1);

  for (const control of form.elements) {
    if (!isFormControl(control)) continue;

    control.addEventListener("input", () => {
      if (control.classList.contains(invalidClass))
        validateControl(control, false);

      revalidateDependents(form, control.name);
      clearFormMessage(form);
    });
    control.addEventListener("change", () => {
      if (control.classList.contains(invalidClass))
        validateControl(control, false);

      revalidateDependents(form, control.name);
      clearFormMessage(form);
    });
  }

  form.addEventListener("reset", () => {
    requestAnimationFrame(() => clearForm(form));
  });
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    clearFormMessage(form);
    if (!validateForm(form)) return;

    void submitForm(form);
  });
}

for (const [index, form] of [
  ...document.querySelectorAll<HTMLFormElement>(formSelector),
].entries()) {
  initForm(form, index);
}
