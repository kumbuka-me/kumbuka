// Opt-in browser performance diagnostics exposed through `kumbuka.perf`.

const storageKey = "kumbuka.performance.enabled";
const timingCookie = "kumbuka_perf";
const measurePrefix = "kumbuka:";
const maxLongTasks = 100;

interface TimingRow {
  name: string;
  durationMs: number;
  startMs: number;
}

interface ResourceRow {
  resource: string;
  type: string;
  durationMs: number;
  transferKB: number;
  decodedKB: number;
  serverTiming: string;
}

interface NavigationReport {
  dnsMs: number;
  connectMs: number;
  tlsMs: number;
  requestToFirstByteMs: number;
  responseDownloadMs: number;
  domInteractiveMs: number;
  domContentLoadedMs: number;
  loadMs: number;
  firstContentfulPaintMs: number | null;
  serverTiming: string;
}

export interface PerformanceReport {
  enabled: boolean;
  navigation: NavigationReport | null;
  initialization: TimingRow[];
  resources: ResourceRow[];
  longTasks: TimingRow[];
  largestContentfulPaintMs: number | null;
}

interface PerformanceConsole {
  enable(): void;
  disable(): void;
  clear(): void;
  report(): PerformanceReport;
  status(): { enabled: boolean; serverTiming: boolean };
}

type KumbukaGlobal = typeof globalThis & {
  kumbuka?: {
    perf?: PerformanceConsole;
    [key: string]: unknown;
  };
};

let initialized = false;
let enabled = false;
let sequence = 0;
let largestContentfulPaintMs: number | null = null;
let longTaskObserver: PerformanceObserver | null = null;
let paintObserver: PerformanceObserver | null = null;
const longTasks: TimingRow[] = [];

// initPerformance installs the console helper and starts collection when it was
// enabled on a previous page load.
export function initPerformance(): void {
  if (initialized || typeof window === "undefined") return;
  initialized = true;
  enabled = storedEnabled();
  installConsole();

  if (enabled) {
    setTimingCookie(true);
    startObservers();
  }
}

// measure records one synchronous Kumbuka initialization step when diagnostics
// are enabled. Disabled diagnostics execute the callback directly.
export function measure<T>(name: string, operation: () => T): T {
  if (!enabled || typeof performance === "undefined") return operation();

  const mark = `${measurePrefix}${name}:start:${++sequence}`;
  performance.mark(mark);
  try {
    return operation();
  } finally {
    performance.measure(`${measurePrefix}${name}`, mark);
    performance.clearMarks(mark);
  }
}

// measureAsync records one asynchronous Kumbuka initialization step when
// diagnostics are enabled.
export async function measureAsync<T>(
  name: string,
  operation: () => Promise<T> | T,
): Promise<T> {
  if (!enabled || typeof performance === "undefined") return operation();

  const mark = `${measurePrefix}${name}:start:${++sequence}`;
  performance.mark(mark);
  try {
    return await operation();
  } finally {
    performance.measure(`${measurePrefix}${name}`, mark);
    performance.clearMarks(mark);
  }
}

function installConsole(): void {
  const root = globalThis as KumbukaGlobal;
  const namespace = (root.kumbuka ??= {});
  namespace.perf = Object.freeze({
    enable: enablePerformance,
    disable: disablePerformance,
    clear: clearPerformance,
    report: reportPerformance,
    status: performanceStatus,
  });
}

function enablePerformance(): void {
  enabled = true;
  storeEnabled(true);
  setTimingCookie(true);
  startObservers();
  console.info(
    "Kumbuka performance diagnostics enabled. Reload once to capture the full navigation and backend Server-Timing data.",
  );
}

function disablePerformance(): void {
  enabled = false;
  storeEnabled(false);
  setTimingCookie(false);
  stopObservers();
  console.info("Kumbuka performance diagnostics disabled.");
}

function clearPerformance(): void {
  if (typeof performance !== "undefined") {
    for (const entry of performance.getEntriesByType("measure")) {
      if (entry.name.startsWith(measurePrefix)) {
        performance.clearMeasures(entry.name);
      }
    }
  }
  longTasks.length = 0;
  largestContentfulPaintMs = null;
  console.info("Kumbuka performance measurements cleared.");
}

function performanceStatus(): { enabled: boolean; serverTiming: boolean } {
  const navigation = navigationEntry();
  return {
    enabled,
    serverTiming: Boolean(
      navigation?.serverTiming.some((timing) => timing.name === "kumbuka"),
    ),
  };
}

function reportPerformance(): PerformanceReport {
  const report = collectReport();

  console.group("Kumbuka performance");
  console.info(
    report.enabled
      ? "Diagnostics are enabled."
      : "Diagnostics are disabled; enable them with kumbuka.perf.enable().",
  );

  if (report.navigation) {
    console.group("Navigation");
    console.table([
      { phase: "DNS", durationMs: report.navigation.dnsMs },
      { phase: "Connect", durationMs: report.navigation.connectMs },
      { phase: "TLS", durationMs: report.navigation.tlsMs },
      {
        phase: "Request → first byte",
        durationMs: report.navigation.requestToFirstByteMs,
      },
      {
        phase: "Response download",
        durationMs: report.navigation.responseDownloadMs,
      },
      {
        phase: "DOM interactive",
        durationMs: report.navigation.domInteractiveMs,
      },
      {
        phase: "DOMContentLoaded",
        durationMs: report.navigation.domContentLoadedMs,
      },
      { phase: "Load", durationMs: report.navigation.loadMs },
      {
        phase: "First contentful paint",
        durationMs: report.navigation.firstContentfulPaintMs,
      },
      {
        phase: "Largest contentful paint",
        durationMs: report.largestContentfulPaintMs,
      },
    ]);
    if (report.navigation.serverTiming) {
      console.info("Backend:", report.navigation.serverTiming);
    }
    if (report.enabled && !performanceStatus().serverTiming) {
      console.info(
        "No navigation Server-Timing data. Reload after enabling diagnostics to include backend timings.",
      );
    }
    console.groupEnd();
  }

  if (report.initialization.length > 0) {
    console.group("Kumbuka initialization (slowest first)");
    console.table(report.initialization);
    console.groupEnd();
  }

  if (report.resources.length > 0) {
    console.group("Resources (25 slowest)");
    console.table(report.resources.slice(0, 25));
    console.groupEnd();
  }

  if (report.longTasks.length > 0) {
    console.group("Long tasks");
    console.table(report.longTasks);
    console.groupEnd();
  }

  console.groupEnd();
  return report;
}

function collectReport(): PerformanceReport {
  const navigation = navigationEntry();
  const firstContentfulPaint = performance
    .getEntriesByName("first-contentful-paint", "paint")
    .at(0);

  return {
    enabled,
    navigation: navigation
      ? {
          dnsMs: rounded(
            navigation.domainLookupEnd - navigation.domainLookupStart,
          ),
          connectMs: rounded(navigation.connectEnd - navigation.connectStart),
          tlsMs: rounded(
            navigation.secureConnectionStart > 0
              ? navigation.connectEnd - navigation.secureConnectionStart
              : 0,
          ),
          requestToFirstByteMs: rounded(
            navigation.responseStart - navigation.requestStart,
          ),
          responseDownloadMs: rounded(
            navigation.responseEnd - navigation.responseStart,
          ),
          domInteractiveMs: rounded(navigation.domInteractive),
          domContentLoadedMs: rounded(navigation.domContentLoadedEventEnd),
          loadMs: rounded(navigation.loadEventEnd || performance.now()),
          firstContentfulPaintMs: firstContentfulPaint
            ? rounded(firstContentfulPaint.startTime)
            : null,
          serverTiming: formatServerTiming(navigation),
        }
      : null,
    initialization: performance
      .getEntriesByType("measure")
      .filter((entry) => entry.name.startsWith(measurePrefix))
      .map((entry) => ({
        name: entry.name.slice(measurePrefix.length),
        durationMs: rounded(entry.duration),
        startMs: rounded(entry.startTime),
      }))
      .sort((left, right) => right.durationMs - left.durationMs),
    resources: performance
      .getEntriesByType("resource")
      .filter(isResourceTiming)
      .map((entry) => ({
        resource: resourceName(entry.name),
        type: entry.initiatorType,
        durationMs: rounded(entry.duration),
        transferKB: rounded(entry.transferSize / 1024),
        decodedKB: rounded(entry.decodedBodySize / 1024),
        serverTiming: formatServerTiming(entry),
      }))
      .sort((left, right) => right.durationMs - left.durationMs),
    longTasks: [...longTasks].sort(
      (left, right) => right.durationMs - left.durationMs,
    ),
    largestContentfulPaintMs,
  };
}

function startObservers(): void {
  if (typeof PerformanceObserver === "undefined") return;

  if (
    longTaskObserver === null &&
    PerformanceObserver.supportedEntryTypes.includes("longtask")
  ) {
    longTaskObserver = new PerformanceObserver((list) => {
      for (const entry of list.getEntries()) {
        longTasks.push({
          name: entry.name || "long task",
          durationMs: rounded(entry.duration),
          startMs: rounded(entry.startTime),
        });
      }
      if (longTasks.length > maxLongTasks) {
        longTasks.splice(0, longTasks.length - maxLongTasks);
      }
    });
    try {
      longTaskObserver.observe({ type: "longtask", buffered: true });
    } catch {
      longTaskObserver.disconnect();
      longTaskObserver = null;
    }
  }

  if (
    paintObserver === null &&
    PerformanceObserver.supportedEntryTypes.includes("largest-contentful-paint")
  ) {
    paintObserver = new PerformanceObserver((list) => {
      const last = list.getEntries().at(-1);
      if (last) largestContentfulPaintMs = rounded(last.startTime);
    });
    try {
      paintObserver.observe({
        type: "largest-contentful-paint",
        buffered: true,
      });
    } catch {
      paintObserver.disconnect();
      paintObserver = null;
    }
  }
}

function stopObservers(): void {
  longTaskObserver?.disconnect();
  paintObserver?.disconnect();
  longTaskObserver = null;
  paintObserver = null;
}

function navigationEntry(): PerformanceNavigationTiming | null {
  if (typeof performance === "undefined") return null;
  const entry = performance.getEntriesByType("navigation").at(0);
  return entry ? (entry as PerformanceNavigationTiming) : null;
}

function isResourceTiming(
  entry: PerformanceEntry,
): entry is PerformanceResourceTiming {
  return entry.entryType === "resource" && "serverTiming" in entry;
}

function formatServerTiming(
  entry: PerformanceNavigationTiming | PerformanceResourceTiming,
): string {
  return entry.serverTiming
    .map((timing) => `${timing.name} ${rounded(timing.duration)} ms`)
    .join(", ");
}

function resourceName(value: string): string {
  try {
    const url = new URL(value, location.href);
    if (url.origin === location.origin) return `${url.pathname}${url.search}`;
    return `${url.origin}${url.pathname}`;
  } catch {
    return value;
  }
}

function storedEnabled(): boolean {
  try {
    return localStorage.getItem(storageKey) === "1";
  } catch {
    return false;
  }
}

function storeEnabled(value: boolean): void {
  try {
    if (value) localStorage.setItem(storageKey, "1");
    else localStorage.removeItem(storageKey);
  } catch {
    // Diagnostics still work for the current page when storage is unavailable.
  }
}

function setTimingCookie(value: boolean): void {
  if (typeof document === "undefined") return;

  const prefix = document.body?.dataset.routePrefix || "";
  const path = prefix ? `${prefix}/` : "/";
  const secure = location.protocol === "https:" ? "; Secure" : "";
  const age = value ? "" : "; Max-Age=0";
  document.cookie = `${timingCookie}=${value ? "1" : ""}; Path=${path}; SameSite=Lax${age}${secure}`;
}

function rounded(value: number): number {
  return Math.round(value * 100) / 100;
}
