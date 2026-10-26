# Frontend Audit — task-033 (8613183..9c334f4)

- **Audit Scope:** 11 changed files under apps/web (hooks, service, type, 5 tests)
- **Guidelines Source:** frontend-dev-guidelines skill; design D11 (design.md:328-373)
- **Date:** 2026-10-26
- **Build:** PASS for `tsc --noEmit` (apps/web, exit 0). `make fe-build` not run; verifier owns it. tools/verify.sh not run by instruction.
- **Tests:** vitest on src/lib/hooks/api, MileageService.test.ts, components/features/vehicles: 39 files, 339 passed, 0 failed. ESLint on hooks/api, MileageService(.test).ts, types/models/mileage.ts: exit 0.
- **Overall:** PASS (no blocking findings)

## File Inventory
- Hook: lib/hooks/api/{mileage,maintenance,fuel,vehicleRecords}.ts
- Service: services/api/MileageService.ts
- Type: types/models/mileage.ts
- Tests: mileage.test.ts, odometerInvalidation.test.tsx (new), vehicleRecords.test.ts, MileageService.test.ts (new), CompleteScheduleDialog.test.tsx
- No pages, components, or schemas changed.

## Anti-Pattern Checklist
| ID | Status | Evidence |
|----|--------|----------|
| FE-01 | PASS | grep `: any\|as any\|<any>` over 11 files: no matches |
| FE-02 | OUT-OF-SCOPE | grep `className=` over 11 files: no matches; no TSX component changed |
| FE-03 | PASS | No component/page changed. Hooks import services (correct layer): fuel.ts:2, mileage.ts:3. Only MileageService.ts:2 imports `lib/api/client` (service layer, allowed). |
| FE-04 | OUT-OF-SCOPE | No components changed; no `z.object` in the 11 files |
| FE-05 | OUT-OF-SCOPE | no `animate-spin` in the 11 files; no components changed |
| FE-06 | PASS | No .tsx component changed; conventions test runs inside the 339-test pass |
| FE-07 | PASS | grep `.push(\|.splice(\|.sort(` over 11 files: no matches |
| FE-08 | PASS | grep `export default` over 11 files: no matches |
| FE-09 | OUT-OF-SCOPE | grep `catch` over 11 files: no matches; no new async error paths |

## Architecture Checklist
| ID | Status | Evidence |
|----|--------|----------|
| FE-10 | PASS | types/models/mileage.ts:4 adds `MileageSource`, :14 narrows `source` to it. Attributes interface is pre-existing. All `source` consumers still compile (tsc exit 0); VehicleRecordDrawer.tsx:399 renders it as text. |
| FE-11 | PASS | MileageService.ts:2 uses `apiClient`; no fetch, no own client. Source param added at :36 (`source=a,b` only when `sources?.length`). Matches D11. |
| FE-12 | PASS | mileage.ts:19-22: `all`/`lists` are `as const`; `list` includes `sources`, so filtered and unfiltered caches differ (asserted at mileage.test.ts:175). |
| FE-13 | OUT-OF-SCOPE | No form component changed |
| FE-14 | OUT-OF-SCOPE | No schema changed |
| FE-18 | OUT-OF-SCOPE | No form changed |

D11 conformance:
- Helper `invalidateOdometerQueries` is at mileage.ts:~28-38 and matches the design body exactly. Home is mileage.ts, as designed. No import cycle: grep for `mileage'` imports in vehicles.ts and dashboard.ts finds none.
- Eight call sites, each present:
  - maintenance.ts: create (:175), update (:194), delete (:270), complete (:409)
  - fuel.ts: create (:90, with vehicleId), update (:104, no vehicleId), delete (:117, no vehicleId)
  - mileage.ts: useCreateMileageRecord onSettled
- Replaced `vehicleKeys.detail` calls are gone from maintenance.ts. The `vehicleKeys` import at maintenance.ts:6 remains; lint and tsc are clean, so it is still used elsewhere in the file.
- vehicleRecords.ts:~57 calls `useMileageRecords({ vehicleId, sources: MANUAL_ONLY })`. The module-level constant keeps the query-key reference stable.
- VehicleTrends, CompleteScheduleDialog and VehicleRecordDrawer are untouched and keep all sources. This matches D11.

## Styling Checklist
| FE-15 | OUT-OF-SCOPE | No interactive elements changed |

## Testing Checklist
| ID | Status | Evidence |
|----|--------|----------|
| FE-16 | PASS | New/updated tests: odometerInvalidation.test.tsx:78-134 (all 8 mutations plus the no-vehicleId fallback), MileageService.test.ts:21-31 (serialize, empty, absent), mileage.test.ts:160-175, vehicleRecords.test.ts:164-169 |
| FE-17 | PASS | MileageService stubs: VehicleRecordDrawer.test.tsx:30, mileage.test.ts:11, odometerInvalidation.test.tsx:30. Only an optional param was added to `listByVehicle`, so existing stubs stay valid. 339 tests pass. |

## Summary

### Blocking (must fix)
- None.

### Non-Blocking (should fix)
- None blocking. Observation: `make fe-build` and `make fe-test` were not run as written; tsc, eslint and the targeted vitest runs stand in for them. The full gate is the verifier's.
