import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
import { describe, it, expect } from 'vitest';

// src/test -> apps/web
const WEB_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../..');
const CSS = readFileSync(resolve(WEB_ROOT, 'src/index.css'), 'utf8');
const TAILWIND = readFileSync(resolve(WEB_ROOT, 'tailwind.config.ts'), 'utf8');

/**
 * The eight properties the shadcn sidebar primitive consumes (FR-TOKEN-1), and
 * the existing token each one mirrors (design §3.1). The mirror map is the
 * point of this file: index.css deliberately uses literal values rather than
 * `var(--card)` aliases, so nothing but a test stops the two drifting.
 */
const MIRRORS: Record<string, string> = {
  '--sidebar': '--card',
  '--sidebar-foreground': '--card-foreground',
  '--sidebar-primary': '--primary',
  '--sidebar-primary-foreground': '--primary-foreground',
  '--sidebar-accent': '--accent',
  '--sidebar-accent-foreground': '--accent-foreground',
  '--sidebar-border': '--border',
  '--sidebar-ring': '--ring',
};

const SIDEBAR_TOKENS = Object.keys(MIRRORS);

/** Custom-property declarations inside one selector block, as name -> value. */
function block(selector: string): Record<string, string> {
  const start = CSS.indexOf(`${selector} {`);
  if (start === -1) throw new Error(`index.css has no ${selector} block`);
  const end = CSS.indexOf('}', start);
  if (end === -1) throw new Error(`index.css ${selector} block is unterminated`);
  const body = CSS.slice(start, end).replace(/\/\*[\s\S]*?\*\//g, '');
  const declarations: Record<string, string> = {};
  for (const line of body.split('\n')) {
    const match = /^\s*(--[\w-]+)\s*:\s*([^;]+);/.exec(line);
    if (match?.[1] && match[2]) declarations[match[1]] = match[2].trim();
  }
  return declarations;
}

const THEMES: Array<{ name: string; declarations: Record<string, string> }> = [
  { name: 'light (:root)', declarations: block(':root') },
  { name: 'dark (.dark)', declarations: block('.dark') },
];

describe('--sidebar-* tokens', () => {
  // FR-TOKEN-1: defined for BOTH themes, not just light.
  for (const theme of THEMES) {
    it(`are all defined in ${theme.name}`, () => {
      const missing = SIDEBAR_TOKENS.filter((token) => !(token in theme.declarations));
      expect(missing).toEqual([]);
    });

    // design §3.1: literal values that mirror the existing palette, so the
    // sidebar reads as the surface it has always been.
    it(`mirror their existing counterparts in ${theme.name}`, () => {
      for (const [token, mirrored] of Object.entries(MIRRORS)) {
        expect(theme.declarations[token], `${token} should mirror ${mirrored}`).toBe(
          theme.declarations[mirrored],
        );
      }
    });

    // FR-TOKEN-2: the active-link and hover states must stay visible against
    // the sidebar surface. --muted and --accent are the SAME value in both
    // themes, which is why the surface mirrors --card and not --muted.
    it(`keep the accent distinguishable from the surface in ${theme.name}`, () => {
      expect(theme.declarations['--sidebar-accent']).not.toBe(theme.declarations['--sidebar']);
    });
  }
});

describe('tailwind.config.ts', () => {
  // Without registration the vendored component's bg-sidebar / border-sidebar-border
  // classes resolve to nothing and the sidebar renders transparent.
  it('registers every --sidebar-* token', () => {
    const missing = SIDEBAR_TOKENS.filter((token) => !TAILWIND.includes(`hsl(var(${token}))`));
    expect(missing).toEqual([]);
  });

  // The shadcn source references `bg-sidebar`, not `bg-sidebar-background`,
  // which only resolves via a DEFAULT key on the nested colour family.
  it('exposes the bare `sidebar` colour via a DEFAULT key', () => {
    expect(TAILWIND).toContain("DEFAULT: 'hsl(var(--sidebar))'");
  });
});
