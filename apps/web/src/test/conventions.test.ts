import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
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
