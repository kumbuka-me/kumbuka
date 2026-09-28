// Request-localized browser messages emitted by the server-rendered layout.

interface BrowserTranslationPayload {
  locale: string;
  messages: Record<string, string>;
}

let cached: BrowserTranslationPayload | null = null;

function translationPayload(): BrowserTranslationPayload {
  if (cached) return cached;
  if (typeof document === "undefined") {
    cached = { locale: "en", messages: {} };
    return cached;
  }

  const source = document.querySelector<HTMLScriptElement>("#kumbuka-i18n");
  if (!source?.textContent?.trim()) {
    cached = { locale: document.documentElement.lang || "en", messages: {} };
    return cached;
  }

  try {
    const parsed = JSON.parse(source.textContent) as unknown;
    if (
      typeof parsed === "object" &&
      parsed !== null &&
      "locale" in parsed &&
      typeof (parsed as { locale?: unknown }).locale === "string" &&
      "messages" in parsed &&
      typeof (parsed as { messages?: unknown }).messages === "object" &&
      (parsed as { messages?: unknown }).messages !== null
    ) {
      cached = parsed as BrowserTranslationPayload;
      return cached;
    }
  } catch {
    // Fall back to authored English copy when embedded locale data is invalid.
  }

  cached = { locale: document.documentElement.lang || "en", messages: {} };
  return cached;
}

// locale returns the effective interface locale selected by the server.
export function locale(): string {
  return translationPayload().locale;
}

// t returns one browser message and interpolates {name} placeholders.
export function t(
  key: string,
  fallback: string,
  values: Record<string, string | number> = {},
): string {
  const message = translationPayload().messages[key] || fallback;
  return message.replace(/\{([a-zA-Z0-9_]+)\}/g, (match, name: string) => {
    const value = values[name];
    return value === undefined ? match : String(value);
  });
}
