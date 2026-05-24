type ClassValue = string | number | null | undefined | false | ClassValue[];

/**
 * Conditional className helper. Joins truthy class values with a space.
 * Kept dependency-free; swap for clsx + tailwind-merge if conflict resolution
 * is needed later.
 */
export function cn(...inputs: ClassValue[]): string {
  const out: string[] = [];
  for (const input of inputs) {
    if (!input) continue;
    if (Array.isArray(input)) {
      const nested = cn(...input);
      if (nested) out.push(nested);
    } else {
      out.push(String(input));
    }
  }
  return out.join(' ');
}
