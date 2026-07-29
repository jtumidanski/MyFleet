import { clsx, type ClassValue } from 'clsx';
import { twMerge } from 'tailwind-merge';

/**
 * Conditional className helper — the shadcn/ui standard. Combines `clsx`
 * (conditional join) with `tailwind-merge` (conflict resolution so later
 * Tailwind classes win over earlier ones).
 */
export function cn(...inputs: ClassValue[]): string {
  return twMerge(clsx(inputs));
}
