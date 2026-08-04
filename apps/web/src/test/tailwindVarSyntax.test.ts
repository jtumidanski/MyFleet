import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
import postcss from 'postcss';
import tailwind from '@tailwindcss/postcss';
import { describe, it, expect } from 'vitest';

// src/test -> apps/web
const WEB_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const ENTRY = resolve(WEB_ROOT, 'src/index.css');

/**
 * Compile the real stylesheet the app ships, through the real PostCSS pipeline.
 *
 * jsdom never evaluates CSS, so no rendering test in this suite can see a
 * declaration the browser drops. The only place this class of bug is visible is
 * the emitted CSS, so that is what this file asserts on.
 */
async function compile(): Promise<string> {
  const result = await postcss([tailwind()]).process(readFileSync(ENTRY, 'utf8'), { from: ENTRY });
  return result.css;
}

describe('compiled Tailwind output', () => {
  /**
   * Tailwind v3 read a square-bracketed bare custom property — `w-` followed by
   * `--sidebar-width` in brackets — as a variable reference, and emitted
   * `width: var(--sidebar-width)`. Tailwind v4 removed that shorthand: brackets
   * now carry a literal value, so the same class emits `width: --sidebar-width`,
   * which is not a valid width and which every browser silently discards. v4
   * spells the reference with parentheses instead: `w-(--sidebar-width)`.
   *
   * The bracket form is deliberately not written out literally anywhere in this
   * file: Tailwind scans test sources too, so an example in a comment would
   * generate the very declaration this test forbids.
   *
   * The failure is silent by construction: the build succeeds, the class is in
   * the stylesheet, and the element just falls back to `auto`. That is how the
   * sidebar's layout spacer collapsed to zero width and let the fixed sidebar
   * overlap the page content after the v4 migration.
   */
  it('never emits a bare custom-property name as a declaration value', async () => {
    const css = await compile();

    // `--x: --y` is legal (custom properties take any token stream); only a
    // *standard* property taking a bare `--name` is the bug.
    const offenders = [...css.matchAll(/(?<![\w-])([a-z-]+)\s*:\s*(--[\w-]+)\s*[;}]/g)]
      .filter((match) => !match[1]!.startsWith('--'))
      .map((match) => `${match[1]}: ${match[2]}`);

    expect([...new Set(offenders)]).toEqual([]);
  });
});
