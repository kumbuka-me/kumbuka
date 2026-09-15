// Runtime guards for values crossing browser trust boundaries.

export type Guard<Value> = (value: unknown) => value is Value;

export function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

export function isStringRecord(
  value: unknown,
): value is Record<string, string> {
  if (!isRecord(value)) return false;

  return Object.values(value).every((item) => typeof item === "string");
}

export function requireArrayOf<Value>(
  value: unknown,
  guard: Guard<Value>,
  description: string,
): Value[] {
  if (!Array.isArray(value) || !value.every(guard))
    throw new Error(`Invalid ${description}.`);

  return value;
}
