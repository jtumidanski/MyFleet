# Vehicle Card Status Banner — Implementation Context

Companion to [`plan.md`](./plan.md). Read this first if you are picking the task up cold: it is the map of what exists today, what the plan assumes, and the handful of facts that are easy to get wrong.

Sources: [`prd.md`](./prd.md) (v1), [`design.md`](./design.md) (approved), [`ux-prototype.html`](./ux-prototype.html).

---

## 1. What this task actually is

Two backend attributes and one rebuilt card.

The backend **already computes** everything the new card needs and throws it away at two return boundaries:

- `ScheduleStatesByVehicle` loads full `QueueRow`s and computes each schedule's `DueState`, then narrows the return to `[]string` — discarding the next-due date and mileage that would let the UI say *how* overdue a vehicle is (`maintenanceschedule/processor.go:163-178`).
- `DeriveStatus` gathers the last-activity timestamp, uses it to compute a status string, and drops it (`vehicle/status.go:45`).

Both gatherers already run once per vehicle inside the list handler's loop (`vehicle/resource.go:52`). **This task adds no database queries** — it widens what those two calls return and surfaces the result as two read-only attributes.

The frontend then replaces the `StatusBadge` chip with a banner that carries the *reason* ("Service overdue by 1,120 mi") rather than a restatement of the status, tinted only on the two statuses that need action.

## 2. Verified facts about the current code

Everything here was read from source in this worktree, not recalled.

### Backend

| Fact | Location |
| --- | --- |
| `DueState` returns `"ok" \| "upcoming" \| "overdue"`; overdue branches run before upcoming branches, which is why the upcoming branches have no upper bound | `maintenanceschedule/recurrence.go:34-51` |
| `DefaultThresholds` = 30 days / 500 miles | `maintenanceschedule/recurrence.go:20` |
| `NextDue` leaves the unused axis at its zero value — `nextDate` zero for mileage-only, `nextMiles` 0 for time-only | `maintenanceschedule/recurrence.go:22-30` |
| `Model.AsSchedule()` projects onto the pure recurrence input; `DueState` always recomputes from it, never from the stored `next_due_*` columns | `maintenanceschedule/model.go:39-48` |
| `ScheduleStatesByVehicle` has exactly **one** consumer | `vehicle/status.go:41` |
| `Provider` has 5 methods; a test fake must implement all of them | `maintenanceschedule/provider.go:24-36` |
| `QueueRow` is `{Schedule Model; CurrentMileage int; FleetID string}` | `maintenanceschedule/provider.go:17-21` |
| `status.Derive` scans for overdue, then upcoming, then the 365-day inactivity check, then Healthy | `status/derive.go:14-27` |
| `server.Resource` is `{Type, ID, Attributes any, Relationships}` — `attributes` is the JSON key | `packages/shared-go/server/jsonapi.go:9-14` |
| `RegisterInputHandler[T any]` binds `data.attributes` into `T`; a named struct works exactly as the current anonymous ones do | `packages/shared-go/server/handler.go:42-55` |
| POST and PATCH bind narrow anonymous structs, which is already why `status` is unwritable | `vehicle/resource.go:61-70`, `:125-129` |
| `dashboard/aggregate.go:214` uses `LastActivityByVehicle` and `ListActiveByFleet` — neither is touched | `dashboard/aggregate.go` |
| There is a working in-memory SQLite harness pattern if a DB-backed test is ever needed | `maintenanceschedule/completion_db_test.go:16-45` |
| `vehicle` tests are in-package, so `Model{id: …, createdAt: …}` struct literals work — the builder has no created-at setter | `vehicle/builder.go`, `vehicle/model.go` |

### Frontend

| Fact | Location |
| --- | --- |
| `VehiclePhotoThumbnail`'s `BOX` is a hardcoded `h-20 w-20 shrink-0 rounded-md` applied to all four states | `VehiclePhotoThumbnail.tsx:11` |
| Its only consumers are `VehicleCard` and its own test — widening the props is safe | grep across `apps/web/src` |
| `useMediaContentUrl(mediaId, 'thumbnail')` is the authenticated object-URL path; a bare `<img src>` cannot be used | `VehiclePhotoThumbnail.tsx:64` |
| `renderWithProviders` already wraps in `MemoryRouter` + `QueryClientProvider` and accepts a `route` option | `src/test/renderWithProviders.tsx` |
| `@testing-library/user-event` ^14.6.1 is available | `apps/web/package.json:35` |
| Tailwind is `^3.4.0`, so the `has-[…]` variant is supported | `apps/web/package.json:45` |
| `Skeleton` applies `animate-pulse rounded-md bg-muted` and is `aria-hidden` | `src/components/ui/skeleton.tsx` |
| `Card` is a plain `<div>` with `rounded-lg border bg-card text-card-foreground shadow-sm`; className merges | `src/components/ui/card.tsx:4-12` |
| `--danger-subtle`, `--danger-subtle-foreground`, `--danger-border` and the `warning-*` set exist in **both** themes | `src/index.css:34-47`, `:79-92` |
| Those map to `bg-danger-subtle` / `text-danger-subtle-foreground` / `border-danger-border` | `tailwind.config.ts:51-73` |
| `formatMileage(n)` returns `"42,000 mi"`; `formatters.ts` has no relative-time helper | `packages/ui-components/src/formatters.ts` |
| `date-fns`, `dayjs`, and `Intl.RelativeTimeFormat` all return **zero** hits across `apps/` and `packages/` | grep |
| lucide icons already proven in this repo: `Car`, `CheckCircle`, `ChevronRight`, `History`, `Moon`, `XCircle`, `Loader2`, `type LucideIcon` | grep across `apps/web/src` |

### Build

| Fact | Location |
| --- | --- |
| `make ci` = `lint-check vet test build fe-test fe-build manifests carfax-template` | `Makefile:48` |
| `make fe-test` runs `apps/web` and `packages/shared-ts` — **not** `packages/ui-components` | `Makefile:23-25` |
| `packages/ui-components` has a `test` script and an existing `formatters.test.ts` that nothing runs | `packages/ui-components/package.json` |
| Node needs `export NVM_DIR="$HOME/.nvm" && . "$NVM_DIR/nvm.sh" && nvm use 22` when `npm` is missing | `CLAUDE.md` |

## 3. Decisions carried in from design, and why

The PRD left six open questions; the design closed all six plus one genuine gap. The plan implements these without revisiting them.

| # | Decision | Why it matters to an implementer |
| --- | --- | --- |
| **D1** | `nextDue` is a **nested object** with `*int` magnitudes | Flattening makes illegal states representable (`axis:"time"` with a `miles`). `*int` not `int`: "due today" is `days == 0`, and `omitempty` on a plain int drops the key |
| **D2** | One monotone `Urgency` scale spanning both states | FR-4.6's "largest normalized breach" is backwards for *upcoming* — it would surface the least imminent schedule. The inversion (`1 - remaining/threshold`) fixes it, and keeps overdue strictly above upcoming so a mixed max needs no filtering |
| **D3** | Magnitude + normalization in `maintenanceschedule`; selection in `vehicle` | Keeps thresholds out of `vehicle` entirely. `vehicle` performs a max with a documented tiebreak and no domain math |
| **D4** | Recompute next-due, **do not** read the stored columns | The stored `next_due_*` are refreshed hourly; `DueState` recomputes. Reading one beside the other can contradict — a schedule completed 20 minutes ago reports `ok` from fresh math while the stored mileage describes the old cycle |
| **D5** | Ship the 320px `thumbnail` variant | Requesting `display` (1280px) for N cards is the exact cost task-005 §4.7 existed to avoid. Softness is a `media-service` variant task |
| **D6** | Zero-guard decouples us from task-006; still land task-006 first | A wiped `created_at` surfaces as an omitted attribute and an em-dash, never an absurd date. Both branches rewrite `vehicle/status.go`, so it is a merge-order matter |
| **D7** | Inactive can never carry due detail | `status.Derive` reaches Inactive only after both scans fall through. Asserted by test, not defended against in code |
| **D8** | `lg:grid-cols-3` unchanged | Whether the taller card wants two wider columns is a judgement against the running UI |
| **D9** | The overlay's loss of text selection is accepted | The alternative is the explicit-buttons layout task-005 shipped and this task replaces |

## 4. Dependency order and why each task is green

Every task ends with a compiling tree and a passing suite. Two things force the ordering:

1. **Retyping `StatusDeps.Schedules` breaks `main.go` and `resource.go` at once.** `vehicleStatusDeps.Schedules = scheduleProc` currently binds by *structural typing* because the gatherer returns `[]string`. With a named struct it cannot, so the adapter, the port change, and the `resource.go` call sites all land in Task 4 together.
2. **Deleting `ScheduleStatesByVehicle` breaks `main.go`.** So Task 2 adds the widened method alongside it and Task 4 deletes the old one, once nothing binds to it.

```
1  DueBreaches + AxisBreach            (maintenanceschedule, purely additive)
2  ScheduleDueByVehicle                (maintenanceschedule, purely additive)
3  Port types + NextDue + selectNextDue (vehicle, purely additive)
4  Derived + Derive + adapter + deletes (vehicle + main.go + processor.go)  ← the pivot
5  Attributes + TransformDerived        (vehicle transport)
─── backend complete; the API now serves the new attributes ───
6  formatRelativeTime + Makefile gate   (packages/ui-components)
7  Types + vehicleBanner                (apps/web, pure)
8  boxClassName override                (VehiclePhotoThumbnail)
9  VehicleCard rebuild                  (the bulk of the work)
10 Skeleton + VehicleList + make ci
```

Tasks 6, 7, and 8 are mutually independent and independent of 1–5; only 9 needs all three.

## 5. The three things most likely to go wrong

**The overlay is easy to get subtly wrong, and jsdom cannot catch it.**
If the Carfax button loses `relative z-10`, or a future wrapper introduces a stacking context, clicking Carfax silently navigates to the detail page instead. It *looks* completely fine. jsdom performs no layout, so a unit test can never prove the stacking works — the plan's tests assert the structural preconditions (the Carfax anchor has no ancestor anchor; the `z-10` and `after:inset-0` classes are present; a click does not change the router location), and the real check lives in `manual-verification.md`. Do not let a green suite substitute for opening a browser here.

**Two struct shapes must stay in sync across the domain boundary.**
`maintenanceschedule.ScheduleDue`/`AxisBreach` and `vehicle.ScheduleDue`/`Breach` are the same shape declared twice, mapped field-for-field in `cmd/main.go`. That duplication is the intended cost of FR-9.5: an alias would make `vehicle` import `maintenanceschedule` transitively. Adding a field means touching three files, and the adapter turns drift into a compile error. Verify the boundary with:

```sh
go list -deps github.com/jtumidanski/myfleet/apps/fleet-service/internal/vehicle | grep maintenanceschedule
```

Expected: no output.

**`omitempty` does nothing on a struct.**
`LastActivityAt` is carried on `Attributes` as a **string**, not a `time.Time`. A `time.Time` field would serialise `"0001-01-01T00:00:00Z"` for the absent case and defeat FR-8.4's "omitted, not zero-valued" contract, which is exactly what the frontend's em-dash and "Status unavailable" paths key on.

## 6. Smaller traps worth knowing

- **`DueBreaches`'s upcoming branches need an explicit upper bound** that `DueState`'s do not. `DueState` reaches its upcoming branches only after the overdue branches returned; `DueBreaches` evaluates each axis independently, so without `!today.After(nd)` a hybrid overdue on mileage would report its time axis as "upcoming in −40 days".
- **Overdue day counts floor at 1**; upcoming day counts may legitimately be **0**, meaning "due today", which has its own copy rule.
- **`Urgency` is computed from the reported integer magnitude**, not the underlying duration, so a test can predict the score from the number the user will see.
- **`toHaveTextContent` does a substring match.** `'/vehicles/v1'` contains `'/'`, so a "did not navigate" assertion written with it passes even when the bug is present. Compare `.textContent` with `toBe`.
- **`Button asChild` renders through a Radix `Slot`**, which merges className onto the child `<a>`. That is why `relative z-10` on the Button lands on the anchor the test inspects.
- **`overflow-hidden` belongs on the photo wrapper, not the card root.** On the root it clips the card link's focus ring (FR-6.8).
- **The prototype tints Healthy with `success-subtle`.** That is its *always-on* grid. The chosen "Conditional" grid uses `.quietrow`, and FR-3.3 is explicit: Healthy and Inactive carry no tint.
- **`packages/ui-components` runs in no test gate today.** Task 6 adds it to `make fe-test` — the same gap the Makefile's own comment says was closed for `shared-ts`.

## 7. Explicitly out of scope

From PRD §2, unchanged: no maintenance **category name** on the card (that is a per-vehicle lookup into a separate domain on a list endpoint — an N+1 needing its own batching design); no photo-upload, deletion, or primary-image changes; no vehicle detail page changes; no `media-service` change; no migrations; no change to status derivation *rules* or thresholds; no grid sorting or filtering; no dashboard changes.

Batching the list handler's pre-existing 2N gather shape is a separate task (NFR-5) — this change neither worsens nor fixes it.

## 8. Definition of done

- All ten tasks committed, each with its tests passing at the time of commit.
- `go list -deps …/vehicle | grep maintenanceschedule` returns nothing.
- `make ci` passes.
- `manual-verification.md` completed against the running app — in particular the Carfax click-through, which no automated test on this branch can prove.
- `superpowers:requesting-code-review` run, findings in `audit.md` addressed.
- task-006 landed first (merge order, not a blocker).
