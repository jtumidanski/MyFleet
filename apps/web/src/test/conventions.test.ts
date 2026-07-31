import { readFileSync, readdirSync, statSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, join, resolve } from 'node:path';
import { describe, it, expect } from 'vitest';

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
    expect(html).toContain("localStorage.getItem('myfleet.theme')");
  });

  it('applies the dark class before the module bundle loads', () => {
    expect(html).toContain("document.documentElement.classList.add('dark')");
    const scriptIndex = html.indexOf("localStorage.getItem('myfleet.theme')");
    const moduleIndex = html.indexOf('type="module"');
    expect(scriptIndex).toBeGreaterThan(-1);
    expect(moduleIndex).toBeGreaterThan(-1);
    expect(scriptIndex).toBeLessThan(moduleIndex);
  });

  // FR-FLASH-1: neither defer nor async, or it stops being pre-paint.
  it('is synchronous', () => {
    const openTag = html.slice(html.lastIndexOf('<script', html.indexOf('myfleet.theme')));
    const firstClose = openTag.slice(0, openTag.indexOf('>'));
    expect(firstClose).not.toContain('defer');
    expect(firstClose).not.toContain('async');
    expect(firstClose).not.toContain('type="module"');
  });

  // FR-FLASH-3: localStorage blocked by privacy settings must not stop the app
  // booting.
  it('is wrapped in try/catch', () => {
    const scriptStart = html.lastIndexOf('<script', html.indexOf('myfleet.theme'));
    const scriptEnd = html.indexOf('</script>', scriptStart);
    const body = html.slice(scriptStart, scriptEnd);
    expect(body).toContain('try {');
    expect(body).toContain('catch');
  });

  // FR-PERF-1: under 500 bytes.
  it('stays small', () => {
    const scriptStart = html.lastIndexOf('<script', html.indexOf('myfleet.theme'));
    const scriptEnd = html.indexOf('</script>', scriptStart);
    const body = html.slice(scriptStart, scriptEnd);
    expect(body.length).toBeLessThan(500);
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

    const offenders = roots
      .flatMap(tsxFiles)
      .filter((file) => file !== self)
      .flatMap((file) =>
        readFileSync(file, 'utf8')
          .split('\n')
          .map((line, index) => ({ file, line: index + 1, text: line }))
          .filter((entry) => PALETTE.test(entry.text)),
      )
      .map((entry) => `${entry.file}:${entry.line}  ${entry.text.trim()}`);

    expect(offenders, `use the semantic tokens in src/index.css instead`).toEqual([]);
  });
});
