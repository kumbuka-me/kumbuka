// Textarea geometry helpers shared by editor autocompletes.

export interface TextareaCaretOffset {
  left: number;
  top: number;
}

// Calculates the caret offset inside a textarea using a hidden mirror element.
export function textareaCaretOffset(
  source: HTMLTextAreaElement,
  caret: number,
): TextareaCaretOffset {
  const computed = getComputedStyle(source);
  const mirror = document.createElement("div");
  const properties = [
    "font-family",
    "font-size",
    "font-weight",
    "font-style",
    "letter-spacing",
    "text-transform",
    "line-height",
    "padding-top",
    "padding-right",
    "padding-bottom",
    "padding-left",
    "border-top-width",
    "border-right-width",
    "border-bottom-width",
    "border-left-width",
  ];

  mirror.style.position = "fixed";
  mirror.style.left = "-10000px";
  mirror.style.top = "0";
  mirror.style.visibility = "hidden";
  mirror.style.whiteSpace = "pre-wrap";
  mirror.style.overflowWrap = "break-word";
  mirror.style.wordBreak = "normal";
  mirror.style.boxSizing = computed.boxSizing;
  mirror.style.width = `${source.offsetWidth}px`;

  for (const property of properties)
    mirror.style.setProperty(property, computed.getPropertyValue(property));

  mirror.textContent = source.value.slice(0, caret);

  const marker = document.createElement("span");

  marker.textContent = source.value.slice(caret, caret + 1) || "\u200b";
  mirror.append(marker);
  document.body.append(mirror);

  const lineHeight =
    Number.parseFloat(computed.lineHeight) ||
    Number.parseFloat(computed.fontSize) * 1.4;
  const offset = {
    left: marker.offsetLeft - source.scrollLeft,
    top: marker.offsetTop - source.scrollTop + lineHeight,
  };

  mirror.remove();
  return offset;
}
