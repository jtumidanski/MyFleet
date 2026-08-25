# Required Field Indicators — Context

Task: `task-028-required-field-indicators`
Worktree: `.worktrees/task-028-required-field-indicators`, branch `task-028-required-field-indicators`
Artifacts: `prd.md` (v1) → `design.md` → `plan.md` (this phase)

---

## What this task is

Every MyFleet form renders bare label text; nothing tells the user which fields
are mandatory until they press Save and `FormMessage` paints red. There is not a
single `aria-required` in `apps/web` today, so screen-reader users get strictly
less than sighted ones. This task adds a `required` flag to the form primitive,
adopts it at 31 call sites, mimics it on the two surfaces that sit outside
react-hook-form, documents the convention, and pins it with a guard test.

Nothing about validation, submission, or any Zod schema changes. Required-ness
is being *communicated*, not redefined.

---

## Key files

### The primitive

- `apps/web/src/components/ui/form.tsx` — `FormItemContext` (`:35-38`),
  `useFormField` (`:41-64`), `FormItem` (`:66-77`), `FormLabel` (`:79-93`),
  `FormControl` (`:95-110`), `FormDescription` (`:112-126`).
  `useFormField()` has **no consumers outside this file** (grep-verified), so
  extending its return shape ripples nowhere.
- `apps/web/src/components/ui/label.tsx` — the shadcn `Label` over
  `@radix-ui/react-label`; `FormLabel` wraps it.
- **New:** `apps/web/src/components/ui/required.tsx` — `RequiredMarker` and
  `RequiredLegend`. Deliberately *not* in `form.tsx`: `MemberList` and
  `PurgeConfirmDialog` need the glyph without the react-hook-form context.

### The ten react-hook-form forms (31 `<FormItem>`s)

| File | Fields |
|---|---|
| `components/features/vehicles/VehicleForm.tsx` | 8 |
| `components/features/vehicles/maintenance/MaintenanceRecordForm.tsx` | 7 |
| `components/features/vehicles/fuel/FuelForm.tsx` | 5 |
| `components/features/vehicles/maintenance/MaintenanceScheduleForm.tsx` | 4 |
| `components/features/vehicles/dialogs/CompleteScheduleDialog.tsx` | 2 |
| `components/features/settings/InviteForm.tsx` | 2 |
| `components/features/vehicles/mileage/MileageForm.tsx` | 1 |
| `components/features/settings/FleetNameForm.tsx` | 1 |
| `pages/OnboardingPage.tsx` | 1 |

(The PRD says "28 call sites"; the count is 31. The per-form tables in
FR-14 … FR-26 are correct — only the total was off.)

### Outside react-hook-form

- `components/features/settings/MemberList.tsx:273-277` — successor-owner
  picker in the leave-fleet dialog, raw `<label>` + Radix `Select`.
- `components/admin/PurgeConfirmDialog.tsx:197-204` — type-to-confirm input,
  shadcn `<Label>` + `<Input>`.

Explicitly untouched (FR-30): `AdminFleetsPage.tsx:105`,
`AdminAuditPage.tsx:73`, `NotificationPreferences.tsx:60`,
`AttachmentPicker.tsx`, `MediaUploadButton.tsx`.

### Schemas (read-only source of truth)

`apps/web/src/lib/schemas/` — `vehicle.ts`, `fuel.ts`, `maintenanceRecord.ts`,
`maintenanceSchedule.ts`, `mileage.ts`, `fleetSettings.ts`. Two carry
cross-field rules in `superRefine`:

- `fuel.ts:29-38` — one of `totalCost` / `pricePerGallon`, message
  `'Provide price per gallon or total cost (or both)'`.
- `maintenanceSchedule.ts:29-47` — `intervalMonths` required for
  `time`/`hybrid`, `intervalMiles` for `mileage`/`hybrid`.

---

## Decisions carried in from design.md

1. **`required` is declared on `FormItem`, not `FormLabel`.** Required-ness is a
   property of the field, and `FormItem` already owns the generated id, the
   description id and the message id. It also makes the guard test one
   unambiguous token per field. Declaring it on *both* was rejected: two
   sources of truth for one boolean, and a call site where they disagree
   renders a marker without `aria-required`.
2. **`text-danger`, not `text-destructive`.** `--destructive` in dark mode is
   `0 62.8% 30.6%` on a `222.2 84% 4.9%` background — roughly 2:1, failing
   both the 4.5:1 and 3:1 thresholds. `--danger` measures 6.67:1 light /
   7.23:1 dark (`docs/tasks/task-003-dark-mode-branding/contrast.md:18,27`).
   `index.css:104-108` records the intended split: status tokens are for text
   on `--background`/`--card`, `--destructive` styles destructive *controls*.
   `text-muted-foreground` was rejected as too easy to miss.
3. **`RequiredMarker` in its own module.** Three consumers (`ui/form.tsx`,
   `MemberList`, `PurgeConfirmDialog`), one definition.
4. **`FuelForm`'s either/or note: one visible `<p>` plus an `sr-only`
   `FormDescription` inside each cost `FormItem`.** The PRD's stated mechanism
   — a single shared `FormDescription` under the pair — does not work:
   `FormDescription` calls `useFormField()`, which throws outside a `FormItem`
   (`form.tsx:52-54`), and its id is scoped to one field. The alternative of
   adding a `describedBy` escape hatch to the primitive was rejected on YAGNI.
5. **Conditional marking binds to the visibility boolean**
   (`required={showMonths}` / `required={showMiles}`, `required={isCreate}`),
   so schema, visibility and marker read as one rule.
6. **The guard test scans source, not the DOM.** What FR-33 protects is a
   convention ("the field named `make` declares itself required"), and a
   rendering version would need providers, category fixtures and router
   context for ten forms. Source scanning is the established idiom in
   `apps/web/src/test/`. Behavioural truth lives in `form.test.tsx` and the two
   dynamic call sites instead.
7. **`InviteForm`'s `role` is marked** even though it opens pre-filled with
   "Member" and its placeholder never renders. Required-ness is a property of
   the field as the schema defines it, not of its initial value.

---

## Discovered during planning — affects implementation

**`getByLabelText` does not honour `aria-hidden`.** Testing Library's
`getLabelContent` (`node_modules/@testing-library/dom/dist/label-helpers.js:11-26`)
concatenates *all* descendant text nodes of a `<label>`. After this change a
required field's label content is `"Make *"`, so an exact
`getByLabelText('Make')` stops matching.

- Regex queries are unaffected (`/email/i`, `/fleet name/i`,
  `/type the fleet name/i`).
- `getByText('Category')` is unaffected — `getNodeText` reads only *direct*
  child text nodes, which is exactly why FR-3 puts the asterisk in a nested
  span.
- Accessible-name queries are unaffected — `dom-accessibility-api` *does* skip
  `aria-hidden` subtrees.

`apps/web/src/pages/VehiclesPage.test.tsx` has six exact `getByLabelText`
queries on `Make` / `Model` / `Year` (`:84-86,107,115,134,203`) and is the only
existing test that breaks. Plan Task 3 moves them to `getByRole('textbox' |
'spinbutton', { name })` — deliberately, not by loosening to a regex. The
`:134` site is a *negative* assertion, so leaving it as a label query would turn
it vacuous (task-019).

New tests written for this task use role+name queries throughout for the same
reason.

---

## Verification

`make fe-test` and `make fe-build` only. No Go service, no `packages/shared-ts`,
no `deploy/k8s`, so no `make ci`, no kustomize render, no `--dry-run=server`.

Node is not always on `PATH`:

```sh
export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22
```

Two checks the suite cannot make:

- `git diff --stat main...HEAD -- apps/web/src/lib/schemas/` must be empty.
- The asterisk must be legible in both themes, including on a field in its
  error state. The token is argued from recorded measurements, but the glyph is
  a few pixels of stroke — look at it.

Per `CLAUDE.md`, run `superpowers:requesting-code-review` (or `/audit-plan`)
before opening the PR; findings go in this folder's `audit.md`.

---

## Dependencies between plan tasks

- Task 1 (primitive) gates everything. It is deliberately behaviour-neutral: no
  call site passes `required` yet, so the full suite must pass after it with
  **zero test edits**. If it does, every later failure is attributable to a
  call site.
- Task 2 (`CategoryCombobox`) must land before Tasks 5 and 6 assert
  `aria-required` on a combobox trigger.
- Tasks 3–8 are independent of one another.
- Task 9 (guard test) depends on all of Tasks 3–8 being complete — it asserts
  whole-object equality per file, so it fails against a partially marked form.
- Task 10 (docs) needs the next free `FE-` number read off
  `.claude/agents/frontend-guidelines-reviewer.md` at implementation time; it
  currently ends at `FE-17`, so the new rule is `FE-18`.
