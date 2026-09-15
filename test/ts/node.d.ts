declare module "node:test" {
  type TestBody = () => void | Promise<void>;
  type Test = (name: string, body: TestBody) => void;

  const test: Test;
  export default test;
}

declare module "node:assert/strict" {
  interface StrictAssert {
    equal(actual: unknown, expected: unknown, message?: string | Error): void;
    deepEqual(
      actual: unknown,
      expected: unknown,
      message?: string | Error,
    ): void;
    ok(value: unknown, message?: string | Error): asserts value;
  }

  const assert: StrictAssert;
  export default assert;
}

declare module "node:fs" {
  export function readFileSync(path: string, encoding: "utf8"): string;
}
declare module "node:vm" {
  export function runInNewContext(code: string, context: object): unknown;
}
