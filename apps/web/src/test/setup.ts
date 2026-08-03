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

// jsdom's matchMedia is a stub that never fires `change`, so theme code
// subscribing to (prefers-color-scheme: dark) cannot be exercised against it.
// This replacement is driven from tests via setPrefersDark, which is what makes
// FR-TEST-5's live-update case testable at all.
const DARK_QUERY = '(prefers-color-scheme: dark)';

type ChangeListener = (event: MediaQueryListEvent) => void;

const mediaListeners = new Set<ChangeListener>();
let mediaPrefersDark = false;

/** Flip the simulated OS appearance and fire `change` at every listener. */
export function setPrefersDark(value: boolean): void {
  if (mediaPrefersDark === value) return;
  mediaPrefersDark = value;
  const event = { matches: value, media: DARK_QUERY } as MediaQueryListEvent;
  mediaListeners.forEach((listener) => listener(event));
}

/** Restore the default (light) and drop any listeners a test left behind. */
export function resetMatchMedia(): void {
  mediaListeners.clear();
  mediaPrefersDark = false;
}

Object.defineProperty(window, 'matchMedia', {
  writable: true,
  configurable: true,
  value: (query: string): MediaQueryList =>
    ({
      get matches() {
        return query === DARK_QUERY ? mediaPrefersDark : false;
      },
      media: query,
      onchange: null,
      addEventListener: (_type: 'change', listener: ChangeListener) => {
        mediaListeners.add(listener);
      },
      removeEventListener: (_type: 'change', listener: ChangeListener) => {
        mediaListeners.delete(listener);
      },
      addListener: () => {},
      removeListener: () => {},
      dispatchEvent: () => false,
    }) as unknown as MediaQueryList,
});

/** Listener count, so a test can assert the provider unsubscribes on unmount. */
export function mediaListenerCount(): number {
  return mediaListeners.size;
}

// jsdom has no layout engine, so it never implements ResizeObserver. cmdk's
// <Command> measures its list with one on mount; without this stub every test
// that renders a Command (e.g. CategoryCombobox) throws "ResizeObserver is not
// defined" before any assertion runs.
class ResizeObserverStub {
  observe(): void {}
  unobserve(): void {}
  disconnect(): void {}
}

Object.defineProperty(globalThis, 'ResizeObserver', {
  value: ResizeObserverStub,
  writable: true,
  configurable: true,
});

// Radix's dropdown/select menus call DOM APIs jsdom does not implement: they
// scroll the highlighted item into view, and they use pointer capture so a
// press-and-drag off the trigger still selects an item. Without these stubs
// every test that opens a DropdownMenu (e.g. ProfileMenu) throws before any
// assertion runs. Same shape as the ResizeObserver stub above: the smallest
// no-op that lets the library's own code path complete.
Element.prototype.scrollIntoView = function scrollIntoView(): void {};
Element.prototype.hasPointerCapture = function hasPointerCapture(): boolean {
  return false;
};
Element.prototype.setPointerCapture = function setPointerCapture(): void {};
Element.prototype.releasePointerCapture = function releasePointerCapture(): void {};
