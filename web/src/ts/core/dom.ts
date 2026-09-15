// DOM helpers for markup that is required by a component template contract.

export function requiredElement<ElementType extends Element>(
  root: ParentNode,
  selector: string,
): ElementType {
  const element = root.querySelector<ElementType>(selector);
  if (!element) throw new Error(`Missing required element: ${selector}`);

  return element;
}

export function requiredElements<ElementType extends Element>(
  root: ParentNode,
  selector: string,
): ElementType[] {
  const elements = [...root.querySelectorAll<ElementType>(selector)];
  if (!elements.length)
    throw new Error(`Missing required elements: ${selector}`);

  return elements;
}

export function requiredAttribute(element: Element, name: string): string {
  const value = element.getAttribute(name)?.trim();
  if (!value) throw new Error(`Missing required attribute: ${name}`);

  return value;
}
