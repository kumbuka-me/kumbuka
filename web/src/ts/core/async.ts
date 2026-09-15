// Small async-control primitives shared by interactive browser features.

export interface LatestRequest {
  next(): AbortSignal;
  current(): AbortSignal | undefined;
  abort(): void;
}

// Creates a request slot where starting a new operation cancels the previous one.
export function createLatestRequest(): LatestRequest {
  let controller: AbortController | undefined;

  return {
    next(): AbortSignal {
      controller?.abort();
      controller = new AbortController();
      return controller.signal;
    },
    current(): AbortSignal | undefined {
      return controller?.signal;
    },
    abort(): void {
      controller?.abort();
      controller = undefined;
    },
  };
}

export interface Debouncer {
  schedule(): void;
  cancel(): void;
}

// Creates a cancellable trailing-edge debounce around one callback.
export function createDebouncer(
  callback: () => void,
  delay: number,
): Debouncer {
  let timer: ReturnType<typeof setTimeout> | undefined;

  return {
    schedule(): void {
      if (timer !== undefined) clearTimeout(timer);

      timer = setTimeout(() => {
        timer = undefined;
        callback();
      }, delay);
    },
    cancel(): void {
      if (timer !== undefined) clearTimeout(timer);
      timer = undefined;
    },
  };
}

// Reports whether an asynchronous operation ended because its signal was aborted.
export function isAbortError(error: unknown): boolean {
  return error instanceof DOMException && error.name === "AbortError";
}
