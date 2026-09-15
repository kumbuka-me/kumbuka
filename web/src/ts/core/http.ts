// Small HTTP response helpers for browser requests.

import { isRecord, isStringRecord } from "./guards.ts";

export interface ProblemPayload {
  error?: string;
  problems?: Record<string, string>;
}

export function followedRedirectURL(
  response: Pick<Response, "redirected" | "url">,
): string | null {
  return response.redirected ? response.url : null;
}

// Validates each field independently so malformed details cannot hide the message.
export function parseProblemPayload(value: unknown): ProblemPayload {
  if (!isRecord(value)) return {};

  return {
    error: typeof value.error === "string" ? value.error : undefined,
    problems: isStringRecord(value.problems) ? value.problems : undefined,
  };
}

export async function responseProblem(
  response: Response,
  payload?: unknown,
): Promise<Error> {
  let candidate = payload;

  if (candidate === undefined) {
    candidate = await response
      .clone()
      .json()
      .catch(() => undefined);
  }

  const problem = parseProblemPayload(candidate);
  const details = Object.entries(problem.problems ?? {})
    .filter(([, message]) => message.trim() !== "")
    .map(([field, message]) => `${field.replaceAll("_", " ")}: ${message}`);
  const message = problem.error?.trim() || `HTTP ${response.status}`;

  return new Error(
    details.length ? `${message}\n\n${details.join("\n")}` : message,
  );
}

// Performs a JSON request without trusting the decoded response shape.
export async function requestJSON(
  input: RequestInfo | URL,
  init?: RequestInit,
): Promise<unknown> {
  const headers = new Headers(init?.headers);
  if (!headers.has("Accept")) headers.set("Accept", "application/json");

  const response = await fetch(input, { ...init, headers });
  if (!response.ok) throw await responseProblem(response);
  if (response.status === 204 || response.status === 205) return undefined;

  try {
    const payload: unknown = await response.json();
    return payload;
  } catch (error) {
    throw new Error("Invalid JSON response.", { cause: error });
  }
}

export function errorMessage(error: unknown): string {
  return error instanceof Error ? error.message : String(error);
}
