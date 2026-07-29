# Household Vehicle Platform — MVP Delivery Roadmap

Created: 2026-05-24
Source of truth: `docs/tasks/task-001-household-vehicle-platform/prd.md` (comprehensive MVP PRD)
Source brief (raw): `docs/household-vehicle-platform-mvp-prd.md`

## Purpose

The MVP PRD intentionally covers the entire platform and is **too large to plan or implement as a
single unit**. This roadmap slices it into dependency-ordered, individually implementable tasks.
Each row below becomes its own `/spec-task` (PRD) → `/design-task` → `/plan-task` → `/execute-task`
cycle, drawing its requirements from the relevant sections of the task-001 PRD.

`task-001` is the **spec/anchor** task (this PRD + roadmap). Actual build work happens in the
sliced tasks below. Numbers are *suggested*; confirm the next free number with
`tools/task-numbers.sh next` when you spec each one.

## Architectural decisions already fixed (apply to every task)

- **D1** `fleet-service` owns all core domain (fleets, vehicles, mileage, maintenance, fuel, dashboards, activity).
- **D2** One PostgreSQL instance, isolated database/schema per service; no cross-service joins.
- **D3** `auth-service` verifies Google OIDC and mints first-party JWTs; services validate them.
- **D4** Single-origin API gateway / reverse proxy routes by path prefix to each service.

## Delivery slices

| # (suggested) | Task | Scope summary | Primary components | Depends on |
|---|---|---|---|---|
| task-002 | **Platform foundation & scaffolding** | Monorepo layout (`/apps`, `/packages`, `/deploy`, `/scripts`, `/docs`); shared packages skeleton (`shared-go`, `shared-ts`, `dto-go`, `ui-components`); docker-compose with PostgreSQL + MinIO + Kafka; reusable Go **service template** (structured logging, OTel, correlation IDs, health + metrics, env config, multi-stage non-root Dockerfile); API-gateway skeleton; web app shell; CI/CD skeleton (PR + main workflows, Gitleaks, formatting); Renovate config. No business features. | all (skeletons), infra | — |
| task-003 | **Authentication (auth-service)** | Google OIDC verification, auto user provisioning, first-party JWT mint + refresh, `/auth/me`; web login flow and authenticated API client wiring; JWT-validation middleware shared lib. | auth-service, web, shared-go, gateway | task-002 |
| task-004 | **Fleets, membership & invites (fleet-service core)** | Fleet CRUD + onboarding (create fleet with name), roles (owner/member/viewer), email invites with expiration + same-email enforcement, member management; **fleet-scoped authorization middleware** (the authz spine everything else reuses). Onboarding + fleet-settings UI. | fleet-service, web, shared-go | task-003 |
| task-005 | **Vehicles & media (media-service)** | Vehicle CRUD, soft-delete + 5-day restore + purge job, vehicle fields; `media-service` (MinIO uploads, metadata, presigned downloads, async thumbnail/display variants); multiple images per vehicle + primary selection. | fleet-service, media-service, web | task-004 |
| task-006 | **Mileage tracking** | Immutable mileage history records; sources (manual now, fuel/maintenance wired later); chronological history, trend graph, latest-mileage auto-fill. | fleet-service, web | task-005 |
| task-007 | **Maintenance system** | Seeded categories; maintenance records (+ document attachments); recurring schedules (time/mileage/hybrid); upcoming + overdue queues with severity; completion → pre-populated record + next-due recompute; recurrence recalculation background job. | fleet-service, web | task-006 |
| task-008 | **Fuel tracking** | Fuel logs (date, mileage, gallons, total cost, price/gal); each entry creates a mileage record; cost/MPG data captured for later aggregation. | fleet-service, web | task-006 |
| task-009 | **Vehicle status & activity/auditing** | Derived vehicle status (Healthy/Upcoming/Overdue/Inactive) with defined logic; fleet activity feed + per-vehicle timeline; domain event production (`vehicle.created`, `maintenance.completed`, `fuel.logged`, `schedule.overdue`, `member.invited`) to Kafka. | fleet-service, web | task-007, task-008 |
| task-010 | **Notifications (notification-service)** | In-app notifications (upcoming/overdue/activity); per-user per-type preferences; idempotent event consumers + scheduled reminder jobs. | notification-service, web | task-009 |
| task-011 | **Dashboard system** | Widget catalog (fleet overview, vehicle status cards, upcoming/overdue maintenance, recent activity, spend by vehicle, mileage trends); add/remove/reorder/resize; per-user layout persistence; aggregation queries. | fleet-service, web | task-009, task-010 |
| task-012 | **Kubernetes deployment & release hardening** | k3s-compatible manifests (raw YAML or Kustomize), ConfigMap/Secret separation, resource requests/limits, readiness/liveness probes, rolling deploys; main-workflow GHCR publishing, version tagging, deployment artifacts, vulnerability scanning. | all, deploy | task-002 (deepens as services land) |

## Sequencing notes

- **Critical path:** task-002 → 003 → 004 establishes scaffolding, identity, and the fleet authz
  spine; nearly everything else depends on task-004's fleet-scoping.
- **Parallelizable after task-006:** task-007 (maintenance) and task-008 (fuel) are independent and
  can run concurrently; both feed task-009.
- **task-012 (K8s/release)** can start its manifest patterns right after task-002 and grow
  incrementally as each service ships, rather than waiting until the end.
- Event-bus wiring is introduced in task-009 (producers) and consumed in task-010; earlier tasks may
  stub event emission behind an interface to avoid rework.
- Re-confirm task numbers at spec time — these are suggestions, not reservations.
