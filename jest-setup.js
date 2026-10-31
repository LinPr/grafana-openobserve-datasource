// Jest setup provided by Grafana scaffolding
import './.config/jest-setup';

// jsdom implements neither IntersectionObserver nor ResizeObserver, but several
// @grafana/ui widgets (Select, Combobox, ...) use them for dropdown positioning
// and measurement. Provide no-op stubs so components can be rendered in tests
// without stubbing every widget.
class ObserverStub {
  readonly root = null;
  readonly rootMargin = '';
  readonly thresholds: readonly number[] = [];

  observe() {}
  unobserve() {}
  disconnect() {}
  takeRecords() {
    return [];
  }
}

if (!('IntersectionObserver' in global)) {
  Object.defineProperty(global, 'IntersectionObserver', { writable: true, value: ObserverStub });
}

if (!('ResizeObserver' in global)) {
  Object.defineProperty(global, 'ResizeObserver', { writable: true, value: ObserverStub });
}
