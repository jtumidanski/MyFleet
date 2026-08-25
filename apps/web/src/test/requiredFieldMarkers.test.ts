import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';
import { dirname, resolve } from 'node:path';
import { describe, it, expect } from 'vitest';

// src/test -> src
const SRC = resolve(dirname(fileURLToPath(import.meta.url)), '..');

/**
 * `true`  — a bare `<FormItem required>`.
 * `false` — no `required` at all.
 * string  — `required={<expr>}`, matched on the exact expression, so swapping a
 *           dynamic flag for a bare one (or the reverse) fails here.
 */
type Expectation = boolean | string;

/**
 * Which field of which form declares itself required. Read off the Zod schemas
 * in lib/schemas/, except where a deviation is annotated below — an
 * undocumented deviation must fail this test, a documented one must not.
 */
const EXPECTED: Record<string, Record<string, Expectation>> = {
  'components/features/vehicles/VehicleForm.tsx': {
    nickname: false,
    // Deviation (FR-15): the schema requires these unconditionally, but in
    // edit mode they render disabled — immutable after create — so the user
    // can neither change them nor fail to supply them, and an asterisk would
    // be noise. Bound to the form's existing isCreate flag.
    make: 'isCreate',
    model: 'isCreate',
    year: 'isCreate',
    trim: false,
    vin: false,
    currentMileage: false,
    notes: false,
  },
  'components/features/vehicles/fuel/FuelForm.tsx': {
    date: true,
    mileage: true,
    gallons: true,
    // Deviation (FR-21): the schema's superRefine requires *one of* these two.
    // Marking both would claim both are mandatory; marking neither would hide
    // the rule. Neither is marked, and the rule is stated in prose instead —
    // visibly under the pair and sr-only inside each FormItem.
    totalCost: false,
    pricePerGallon: false,
  },
  'components/features/vehicles/maintenance/MaintenanceRecordForm.tsx': {
    categoryId: true,
    performedAt: true,
    description: false,
    mileage: false,
    cost: false,
    vendor: false,
    notes: false,
  },
  'components/features/vehicles/maintenance/MaintenanceScheduleForm.tsx': {
    categoryId: true,
    recurrenceType: true,
    // Deviation (FR-19): required only for some recurrence types. Bound to the
    // same booleans that decide whether the field renders at all, so the
    // schema's superRefine, the visibility rule and the marker are one rule.
    intervalMonths: 'showMonths',
    intervalMiles: 'showMiles',
  },
  'components/features/vehicles/mileage/MileageForm.tsx': {
    mileage: true,
  },
  'components/features/vehicles/dialogs/CompleteScheduleDialog.tsx': {
    date: true,
    // Auto-filled from the latest mileage record and clearable (FR-23).
    latestMileage: false,
  },
  'components/features/settings/FleetNameForm.tsx': {
    name: true,
  },
  'components/features/settings/InviteForm.tsx': {
    email: true,
    // Marked despite opening pre-filled with "member": required-ness is a
    // property of the field as the schema defines it, not of its initial value.
    role: true,
  },
  'pages/OnboardingPage.tsx': {
    name: true,
  },
};

/** Forms with three or more rendered fields carry the legend (FR-10/FR-11). */
const EXPECTED_LEGEND: Record<string, boolean> = {
  'components/features/vehicles/VehicleForm.tsx': true,
  'components/features/vehicles/fuel/FuelForm.tsx': true,
  'components/features/vehicles/maintenance/MaintenanceRecordForm.tsx': true,
  'components/features/vehicles/maintenance/MaintenanceScheduleForm.tsx': true,
  'components/features/vehicles/mileage/MileageForm.tsx': false,
  'components/features/vehicles/dialogs/CompleteScheduleDialog.tsx': false,
  'components/features/settings/FleetNameForm.tsx': false,
  'components/features/settings/InviteForm.tsx': false,
  'pages/OnboardingPage.tsx': false,
};

function read(file: string): string {
  return readFileSync(resolve(SRC, file), 'utf8');
}

/** Accepts both `<FormItem required>` and `<FormItem required={expr}>`. */
function declaredRequired(block: string): Expectation {
  const match = /<FormItem\s+required(?:=\{([^}]*)\})?(?:\s|>)/.exec(block);
  if (!match) return false;
  return match[1] === undefined ? true : match[1].trim();
}

describe('required field markers', () => {
  // Every <FormField> in these files is a flat sibling — none nests another —
  // so splitting on the tag yields one block per field.
  it.each(Object.keys(EXPECTED))('%s declares each field as specified', (file) => {
    const blocks = read(file).split('<FormField').slice(1);
    const actual: Record<string, Expectation> = {};

    for (const block of blocks) {
      const name = /name="([^"]+)"/.exec(block)?.[1];
      expect(name, `a <FormField> in ${file} has no static name=""`).toBeTruthy();
      actual[name as string] = declaredRequired(block);
    }

    // Whole-object equality, so a newly added field nobody classified fails
    // loudly instead of passing by omission.
    expect(actual).toEqual(EXPECTED[file]);
  });

  it.each(Object.entries(EXPECTED_LEGEND))('%s legend presence is %s', (file, expected) => {
    expect(read(file).includes('<RequiredLegend />')).toBe(expected);
  });

  // The two surfaces outside react-hook-form get the same treatment by hand.
  it('marks the successor picker in the leave-fleet dialog', () => {
    const source = read('components/features/settings/MemberList.tsx');
    const start = source.indexOf('htmlFor="successor"');
    expect(start).toBeGreaterThan(-1);
    const region = source.slice(start, source.indexOf('</Select>', start));

    expect(region).toContain('<RequiredMarker />');
    expect(region).toContain('aria-required="true"');
  });

  it('marks the purge confirmation input', () => {
    const source = read('components/admin/PurgeConfirmDialog.tsx');
    const start = source.indexOf('htmlFor="purge-confirmation"');
    expect(start).toBeGreaterThan(-1);
    // Delimited by the wrapping </div>, not by the first `/>` — the marker is
    // itself a self-closing tag and would truncate the region before the Input.
    const region = source.slice(start, source.indexOf('</div>', start));

    expect(region).toContain('<RequiredMarker />');
    expect(region).toContain('aria-required="true"');
  });
});
