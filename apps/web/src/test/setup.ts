import '@testing-library/jest-dom/vitest';

// Node 22+ exposes an experimental built-in `localStorage` global that shadows
// jsdom's implementation and throws without a backing file. Install a simple
// in-memory polyfill so storage-backed code works deterministically in tests.
class MemoryStorage implements Storage {
  private store = new Map<string, string>();
  get length(): number {
    return this.store.size;
  }
  clear(): void {
    this.store.clear();
  }
  getItem(key: string): string | null {
    return this.store.has(key) ? (this.store.get(key) as string) : null;
  }
  key(index: number): string | null {
    return Array.from(this.store.keys())[index] ?? null;
  }
  removeItem(key: string): void {
    this.store.delete(key);
  }
  setItem(key: string, value: string): void {
    this.store.set(key, String(value));
  }
}

Object.defineProperty(globalThis, 'localStorage', {
  value: new MemoryStorage(),
  writable: true,
  configurable: true,
});
