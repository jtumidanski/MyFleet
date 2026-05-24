/**
 * Task 15.7 — Dashboard widget system tests.
 *
 * REQUIRED TEST: The widget registry maps every backend catalog type to a
 * component (no missing entries). Iterates ValidCatalog and asserts each
 * has a registry entry.
 *
 * We test via widgetManifest.ts (pure TS, no React imports) to keep tests
 * runnable in the unit-test environment (jsdom, no JSX transform needed).
 * widgetRegistry.tsx imports the same manifest and adds the component refs —
 * the compile-time exhaustiveness check in widgetManifest.ts ensures they stay
 * in sync.
 */
import { describe, it, expect } from 'vitest';
import { dashboardKeys } from './dashboard';
import { widgetManifest, WIDGET_CATALOG } from '../../../components/features/dashboard/widgetManifest';

// ---------------------------------------------------------------------------
// dashboardKeys hierarchy
// ---------------------------------------------------------------------------

describe('dashboardKeys', () => {
  it('is hierarchical', () => {
    expect(dashboardKeys.all).toEqual(['dashboards']);
    expect(dashboardKeys.layout('f1')).toEqual(['dashboards', 'layout', 'f1']);
    expect(dashboardKeys.overview('f1')).toEqual(['dashboards', 'overview', 'f1']);
    expect(dashboardKeys.spendByVehicle('f1', { from: '2024-01-01', to: '2024-12-31' })).toEqual([
      'dashboards',
      'spendByVehicle',
      'f1',
      { from: '2024-01-01', to: '2024-12-31' },
    ]);
    expect(dashboardKeys.mileageTrends('v1', { from: '', to: '' })).toEqual([
      'dashboards',
      'mileageTrends',
      'v1',
      { from: '', to: '' },
    ]);
  });
});

// ---------------------------------------------------------------------------
// REQUIRED: widget manifest covers all catalog types (no missing entries)
// ---------------------------------------------------------------------------

describe('widgetManifest', () => {
  it('covers every entry in WIDGET_CATALOG (no missing types)', () => {
    for (const widgetType of WIDGET_CATALOG) {
      expect(
        widgetManifest[widgetType],
        `widgetManifest is missing entry for catalog type "${widgetType}"`,
      ).toBeDefined();
    }
  });

  it('WIDGET_CATALOG has 7 entries matching the backend ValidCatalog', () => {
    const expected = [
      'fleet-overview',
      'vehicle-status',
      'upcoming-maintenance',
      'overdue-maintenance',
      'recent-activity',
      'spend-by-vehicle',
      'mileage-trends',
    ];
    expect(WIDGET_CATALOG).toHaveLength(7);
    for (const t of expected) {
      expect(WIDGET_CATALOG).toContain(t);
    }
  });

  it('each manifest entry has label, defaultWidth, defaultHeight', () => {
    for (const widgetType of WIDGET_CATALOG) {
      const entry = widgetManifest[widgetType];
      expect(typeof entry.label).toBe('string');
      expect(entry.label.length).toBeGreaterThan(0);
      expect(typeof entry.defaultWidth).toBe('number');
      expect(typeof entry.defaultHeight).toBe('number');
    }
  });
});
