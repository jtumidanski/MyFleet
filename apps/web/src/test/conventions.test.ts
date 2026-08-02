import { readFileSync, readdirSync, statSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join, resolve } from 'node:path';
import { describe, it, expect } from 'vitest';
import { THEME_STORAGE_KEY } from '../lib/theme';
import { MEDIA_QUERY } from '../context/ThemeContext';

// src/test -> apps/web
const WEB_ROOT = resolve(dirname(fileURLToPath(import.meta.url)), '../..');

// FR-TEST-8. The pre-paint script cannot be shared with src/lib/theme.ts — a
// module import in <head> is asynchronous by definition and would reintroduce
// exactly the flash it exists to prevent. This test is the mitigation for that
// duplication: a future cleanup that "removes the redundant inline script"
// fails here instead of silently regressing every page load.
describe('index.html pre-paint theme script', () => {
  const html = readFileSync(resolve(WEB_ROOT, 'index.html'), 'utf8');

  it('is present and reads the shared storage key', () => {
    expect(html).toContain(`localStorage.getItem('${THEME_STORAGE_KEY}')`);
  });

  it('applies the dark class before the module bundle loads', () => {
    expect(html).toContain("document.documentElement.classList.add('dark')");
    const scriptIndex = html.indexOf(`localStorage.getItem('${THEME_STORAGE_KEY}')`);
    const moduleIndex = html.indexOf('type="module"');
    expect(scriptIndex).toBeGreaterThan(-1);
    expect(moduleIndex).toBeGreaterThan(-1);
    expect(scriptIndex).toBeLessThan(moduleIndex);
  });

  // FR-FLASH-1: neither defer nor async, or it stops being pre-paint.
  it('is synchronous', () => {
    const openTag = html.slice(html.lastIndexOf('<script', html.indexOf(THEME_STORAGE_KEY)));
    const firstClose = openTag.slice(0, openTag.indexOf('>'));
    expect(firstClose).not.toContain('defer');
    expect(firstClose).not.toContain('async');
    expect(firstClose).not.toContain('type="module"');
  });

  // FR-FLASH-3: localStorage blocked by privacy settings must not stop the app
  // booting.
  it('is wrapped in try/catch', () => {
    const scriptStart = html.lastIndexOf('<script', html.indexOf(THEME_STORAGE_KEY));
    const scriptEnd = html.indexOf('</script>', scriptStart);
    const body = html.slice(scriptStart, scriptEnd);
    expect(body).toContain('try {');
    expect(body).toContain('catch');
  });

  // FR-PERF-1: under 500 bytes.
  it('stays small', () => {
    const scriptStart = html.lastIndexOf('<script', html.indexOf(THEME_STORAGE_KEY));
    const scriptEnd = html.indexOf('</script>', scriptStart);
    const body = html.slice(scriptStart, scriptEnd);
    expect(body.length).toBeLessThan(500);
  });

  // Pins the pre-paint script's media query against ThemeContext's MEDIA_QUERY
  // so the two cannot drift. Scoped to the script body specifically — the
  // theme-color <meta> tags also contain this substring, so checking the
  // whole file would pass even if only the script's copy went stale.
  it('uses the same media query as ThemeContext', () => {
    const scriptStart = html.lastIndexOf('<script', html.indexOf(THEME_STORAGE_KEY));
    const scriptEnd = html.indexOf('</script>', scriptStart);
    const body = html.slice(scriptStart, scriptEnd);
    expect(body).toContain(MEDIA_QUERY);
  });
});

// The generator emits the path into BOTH brandMarkPath.ts and favicon.svg
// (design §8.2). Cheap insurance against someone hand-editing one and not the
// other, which would silently give the tab a different mark from the sidebar.
describe('brand mark', () => {
  it('is identical in brandMarkPath.ts and favicon.svg', () => {
    const ts = readFileSync(resolve(WEB_ROOT, 'src/components/brandMarkPath.ts'), 'utf8');
    const svg = readFileSync(resolve(WEB_ROOT, 'public/favicon.svg'), 'utf8');

    const match = /'([^']+)'/.exec(ts);
    if (!match) throw new Error('brandMarkPath.ts should export a single-quoted path string');
    expect(svg).toContain(match[1]);
  });
});

describe('index.html icon wiring', () => {
  const html = readFileSync(resolve(WEB_ROOT, 'index.html'), 'utf8');

  it('declares the SVG favicon with the ICO as an explicit alternate', () => {
    // rel="alternate icon" so SVG-capable browsers prefer the vector
    // (FR-ICON-5).
    expect(html).toContain('<link rel="icon" href="/favicon.svg" type="image/svg+xml" />');
    expect(html).toContain('rel="alternate icon"');
  });

  it('declares the apple-touch icon and the manifest', () => {
    expect(html).toContain('rel="apple-touch-icon"');
    expect(html).toContain('rel="manifest"');
  });

  // FR-ICON-8: the manifest format has no media-query support, so per-theme
  // browser chrome comes from these two metas instead. The values are the
  // rendered equivalents of the --background tokens; nothing enforces that
  // coupling, so it is recorded in the design's deployment notes.
  it('declares both theme-color metas', () => {
    expect(html).toContain('media="(prefers-color-scheme: light)" content="#ffffff"');
    expect(html).toContain('media="(prefers-color-scheme: dark)" content="#020817"');
  });
});

// FR-CONVERT-10 / FR-TEST-9. Hardcoded palette classes are how dark mode rots:
// each one renders light-on-light or as an unreadable smear once the background
// goes dark, and nothing in the type system stops a new one appearing.
describe('no hardcoded palette classes', () => {
  const PALETTE =
    /(bg|text|border|ring|divide)-(gray|slate|zinc|neutral|white|black|red|green|blue|amber|yellow|emerald|orange)/;

  /**
   * A deliberate, narrow allowlist of two lines — not a loophole for palette
   * classes in general. Task 012's dialog/sheet overlay scrim needs a
   * translucent black backdrop (Radix's own docs use it; it's the standard
   * convention for dimming page content behind a modal in *either* theme, not
   * a per-theme accent this project tokenizes). No `--overlay` token exists
   * in index.css, and the PALETTE regex has no way to distinguish "modal
   * scrim" from "a stray gray-500 someone typed instead of a token" — so
   * rather than weaken the regex (which would stop catching real
   * regressions) this exempts exactly these two known, reviewed lines by
   * file + exact text. Any other black/gray/etc. usage — including a NEW
   * line in these same two files — still fails loudly.
   */
  const ALLOWED_PALETTE_USAGES: ReadonlyArray<{ file: string; text: string }> = [
    { file: 'src/components/ui/dialog.tsx', text: "'fixed inset-0 z-50 bg-black/80" },
    { file: 'src/components/ui/sheet.tsx', text: "'fixed inset-0 z-50 bg-black/80" },
  ];

  function tsxFiles(dir: string): string[] {
    return readdirSync(dir).flatMap((entry) => {
      const full = join(dir, entry);
      if (statSync(full).isDirectory()) return tsxFiles(full);
      return full.endsWith('.tsx') ? [full] : [];
    });
  }

  it('are absent from apps/web/src and packages/ui-components/src', () => {
    const roots = [resolve(WEB_ROOT, 'src'), resolve(WEB_ROOT, '../../packages/ui-components/src')];
    // This file necessarily contains the pattern. It is a .ts, and the scan is
    // .tsx-only, so it is out of scope by construction — the explicit skip is
    // belt-and-braces against a future rename.
    const self = fileURLToPath(import.meta.url);

    const filesByRoot = roots.map((root) => ({ root, files: tsxFiles(root) }));
    // A root that silently resolves to zero files (a moved directory, a
    // rename) would make the assertion below pass trivially — StatusBadge.tsx
    // is the one file make fe-test never covers on its own, so an empty walk
    // into packages/ui-components/src must fail loudly, not pass silently.
    filesByRoot.forEach(({ root, files }) => {
      expect(files.length, `expected at least one .tsx file under ${root}`).toBeGreaterThan(0);
    });

    const offenders = filesByRoot
      .flatMap(({ files }) => files)
      .filter((file) => file !== self)
      .flatMap((file) =>
        readFileSync(file, 'utf8')
          .split('\n')
          .map((line, index) => ({ file, line: index + 1, text: line }))
          .filter((entry) => PALETTE.test(entry.text))
          .filter(
            (entry) =>
              !ALLOWED_PALETTE_USAGES.some(
                (allowed) => entry.file.endsWith(allowed.file) && entry.text.includes(allowed.text),
              ),
          ),
      )
      .map((entry) => `${entry.file}:${entry.line}  ${entry.text.trim()}`);

    expect(offenders, `use the semantic tokens in src/index.css instead`).toEqual([]);
  });
});

// FR-17. Every authenticated page's title goes through PageHeader; a
// hand-written <h1> is how the Dashboard drifted to text-lg and an <h2> in the
// first place. The unauthenticated pages are centered-card layouts outside the
// AppLayout shell — LoginPage's hero <h1> is a deliberate exception, not drift.
describe('authenticated pages do not hand-write their title', () => {
  const UNAUTHENTICATED = ['LoginPage.tsx', 'OnboardingPage.tsx', 'InviteAcceptPage.tsx'];

  it('contain no <h1> element', () => {
    const pagesDir = resolve(WEB_ROOT, 'src/pages');

    const offenders = readdirSync(pagesDir)
      .filter((f) => f.endsWith('.tsx') && !f.endsWith('.test.tsx'))
      .filter((f) => !UNAUTHENTICATED.includes(f))
      .flatMap((file) =>
        readFileSync(join(pagesDir, file), 'utf8')
          .split('\n')
          .map((text, index) => ({ file, line: index + 1, text }))
          .filter((entry) => entry.text.includes('<h1')),
      )
      .map((entry) => `${entry.file}:${entry.line}  ${entry.text.trim()}`);

    expect(offenders, 'render the page title via <PageHeader title="…" /> instead').toEqual([]);
  });
});
