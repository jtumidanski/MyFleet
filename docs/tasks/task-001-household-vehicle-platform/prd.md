# Household Vehicle Management Platform — MVP Product Requirements Document

Version: v1
Status: Draft
Created: 2026-05-24
---

> **Scope note.** Per explicit direction, this single PRD covers the **entire MVP** of the
> Household Vehicle Management Platform rather than one feature slice. It is the canonical,
> decision-resolved version of the source brief (preserved at
> `docs/household-vehicle-platform-mvp-prd.md`). The MVP is intentionally large; an ordered
> breakdown into implementable follow-on tasks lives in `docs/roadmap.md`. Treat this document
> as the contract for *what* the platform must do; service-internal *how* decisions are deferred
> to each task's `/design-task` phase except where this PRD fixes a cross-cutting architectural
> decision (see §7).

## 1. Overview

The Household Vehicle Management Platform is a cloud-hosted, collaborative system for households
to manage the vehicles they own together. It centers on four jobs: tracking maintenance history,
scheduling recurring maintenance (time-, mileage-, and hybrid-based), giving dashboard-driven
operational visibility into what is upcoming or overdue, and preserving historical records,
receipts, and documents over the long term.

The platform is multi-tenant around the concept of a **fleet** — an independent household with one
or more collaborating users sharing a collection of vehicles. All data is fleet-scoped, and
authorization is enforced on every request against the caller's membership and role in the
fleet that owns the data.

The MVP deliberately prioritizes practical utility and data quality over AI-first workflows.
It establishes a structured data foundation (mileage history, standardized maintenance
categories, an event-driven backbone) that later roadmap phases — OCR, VIN decoding, predictive
maintenance — can build on without re-modeling. The product experience is calm, operationally
focused, and desktop-first while remaining responsive on mobile browsers.

## 2. Goals

Primary goals:
- Provide a collaborative, multi-user household vehicle management platform (fleets).
- Track per-vehicle maintenance history and recurring maintenance requirements.
- Support shared household ownership workflows with roles (owner / member / viewer).
- Provide dashboard visibility into upcoming and overdue maintenance and fleet activity.
- Preserve historical records, documents, and receipts with soft-delete and recovery windows.
- Establish structured, event-driven data foundations for future intelligent workflows.

Secondary goals:
- Support enthusiast-oriented organization and long-term history retention.
- Provide operational audit visibility (who did what, when).
- Establish reusable platform foundations: shared libraries, CI/CD, container/K8s deployment.

Non-goals (explicitly out of scope for the MVP):
- Native mobile applications (responsive web only).
- OCR receipt ingestion; AI maintenance extraction; predictive maintenance AI.
- Mechanic marketplace integrations; OBD / vehicle telemetry integrations.
- Advanced financial analytics; VIN decoding integrations.
- Real-time collaborative editing; social / community features.
- Email and push notifications (in-app notifications only for MVP).
- Arbitrary BI / ad-hoc query tooling (predefined dashboard widgets only).

## 3. User Stories

Authentication & onboarding
- As a new user, I want to sign in with my Google account so that I can start without creating a password.
- As a new user, I want to create a fleet with a name during onboarding so that I have a household to manage vehicles in.

Fleet collaboration
- As a fleet owner, I want to invite household members by email so that we can collaborate on the same vehicles.
- As a fleet owner, I want to assign roles (member, viewer) so that I control who can edit versus only view.
- As a fleet owner, I want to remove members and rename the fleet so that I can keep the household accurate.
- As an invited user, I want an invite tied to my email so that only the intended person can accept it.

Vehicles & media
- As a fleet member, I want to add, edit, and soft-delete vehicles so that the fleet reflects what we own.
- As a fleet owner, I want to restore a recently deleted vehicle so that accidental deletions are recoverable.
- As a fleet member, I want to upload multiple images per vehicle and pick a primary so that vehicles are recognizable.

Mileage
- As a fleet member, I want mileage to be tracked over time from fuel, maintenance, and manual updates so that I can see trends and the latest reading auto-fills forms.

Maintenance
- As a fleet member, I want to log maintenance with date, mileage, cost, provider, and notes, and attach receipts, so that we keep a complete history.
- As a fleet member, I want to define recurring maintenance (e.g. oil change every 5,000 mi or 12 months) so that the system reminds us when it is due.
- As a fleet member, I want a queue of upcoming and overdue maintenance with severity so that I know what needs attention.
- As a fleet member, I want completing a scheduled item to pre-fill a maintenance record so that completion is one clean step.

Fuel
- As a fleet member, I want to log fuel (date, mileage, gallons, total cost, price/gal) so that mileage updates and future MPG/cost analysis is possible.

Dashboard & visibility
- As a user, I want a customizable dashboard (add/remove/reorder/resize widgets) so that I see what matters to me.
- As a user, I want vehicle status derived automatically (Healthy / Upcoming / Overdue / Inactive) so that I can triage at a glance.

Activity & notifications
- As a fleet member, I want a fleet activity feed and per-vehicle timeline so that I can see collaborative history.
- As a user, I want in-app notifications for upcoming/overdue maintenance and fleet activity, configurable per type, so that I stay informed without noise.

## 4. Functional Requirements

### 4.1 Authentication & Identity
- FR-AUTH-1: Google OAuth (OIDC) is the **only** authentication provider for the MVP. The
  architecture must remain OIDC-compatible so additional providers can be added later.
- FR-AUTH-2: User accounts are auto-provisioned on first successful Google sign-in, keyed by the
  Google subject (`sub`); email and basic profile (name, avatar) are captured.
- FR-AUTH-3: After verifying the Google identity, `auth-service` issues a **first-party signed JWT**
  (access token) plus a refresh mechanism; all other services authenticate by validating that JWT.
  (See §7 for the cross-cutting decision.)
- FR-AUTH-4: JWT access tokens carry at minimum: user id, email, and active-fleet context with
  the user's role in that fleet. Tokens expire; a refresh flow issues new access tokens without
  re-running the Google login.
- FR-AUTH-5: A user may **actively belong to exactly one fleet** during the MVP, but the data model
  must support many-to-many user↔fleet membership for the future.

### 4.2 Fleet & Membership
- FR-FLEET-1: A user creates a fleet during onboarding; a fleet requires a user-defined name. The
  system may suggest a default name (e.g. "<Name>'s Household").
- FR-FLEET-2: Roles and permissions:
  - **Owner**: invite users, remove users, rename fleet, manage vehicles, restore deleted vehicles, plus all member abilities.
  - **Member**: add/edit maintenance records, add fuel logs, upload documents, manage schedules, manage vehicles' non-destructive fields.
  - **Viewer** (optional role): read-only access to all fleet data; no writes.
- FR-FLEET-3: Invitations are email-based, carry a target role, and have an expiration. An invite
  can only be accepted by an authenticated user **whose Google email matches** the invited email
  (same-email enforcement). Expired or already-accepted invites are rejected.
- FR-FLEET-4: Owners can revoke pending invites and remove existing members (an owner cannot remove
  themselves if they are the sole owner).

### 4.3 Vehicle Management
- FR-VEH-1: Full CRUD — add, edit, soft-delete, restore.
- FR-VEH-2: Vehicle fields: nickname/display name (optional), make, model, trim (optional), year,
  VIN (optional), current mileage, primary image, notes.
- FR-VEH-3: Multiple images per vehicle with a single selectable primary image; image upload is
  delegated to `media-service`, which performs async thumbnail generation and resizing/compression.
- FR-VEH-4: Soft-deleted vehicles remain recoverable for **5 days**; an async cleanup job
  permanently purges entities past their recovery window. Restore is owner-only.

### 4.4 Mileage Tracking
- FR-MILE-1: Mileage is tracked via dedicated, immutable **mileage history records**, never as a
  single mutable field alone. The vehicle's `current_mileage` reflects the latest record.
- FR-MILE-2: Mileage records may originate from fuel logs, maintenance records, or manual updates;
  each record retains its source and a reference to the originating entity.
- FR-MILE-3: Mileage history must preserve chronological order and support graphing, projections,
  and audit visibility. The latest known mileage auto-populates relevant forms.

### 4.5 Maintenance System
- FR-MAINT-1: System-defined, standardized maintenance **categories** are seeded (e.g. Oil Change,
  Tire Rotation, Brake Service). Categories are structured for later analytics.
- FR-MAINT-2: Maintenance **records** support: date, mileage, cost, provider/shop, notes, category,
  and attached documents/receipts. Records are addable and editable and associated with a vehicle.
- FR-MAINT-3: Recurring maintenance supports three recurrence modes: **time-based** (every N months),
  **mileage-based** (every N miles), and **hybrid** (whichever comes first), e.g. "oil change every
  5,000 mi OR 12 months", "tire rotation every 7,500 mi".
- FR-MAINT-4: Scheduling surfaces an **upcoming queue** and **overdue tracking**, each item carrying
  a severity level: Informational, Recommended, Urgent.
- FR-MAINT-5: Completing a scheduled item transitions it cleanly into a historical maintenance
  record, pre-populating known values (date, latest mileage, category), and recomputes the next
  due date/mileage for the schedule.
- FR-MAINT-6: Due/overdue state is recomputed when mileage changes and on a periodic background job.

### 4.6 Fuel Tracking
- FR-FUEL-1: Fuel entries support date, mileage, gallons, total cost, and price per gallon (the
  system may derive price/gal from total/gallons or vice versa when one is omitted).
- FR-FUEL-2: A fuel entry creates a mileage history record and feeds future MPG calculations and
  cost aggregation. (MPG display is a dashboard concern; raw data must be captured now.)

### 4.7 Dashboard System
- FR-DASH-1: Users can add, remove, reorder, and resize widgets on their dashboard. Layout is
  per-user (each user customizes their own view of a fleet).
- FR-DASH-2: Initial widget catalog: Fleet overview, Vehicle status cards, Upcoming maintenance,
  Overdue maintenance, Recent activity feed, Spend by vehicle, Mileage trends.
- FR-DASH-3: Only predefined widgets are available; no arbitrary BI/query tooling in the MVP.

### 4.8 Vehicle Status (derived)
- FR-STATUS-1: Vehicle status is **derived automatically**, never set manually. Supported statuses:
  Healthy, Upcoming Maintenance, Overdue, Inactive.
- FR-STATUS-2: Derivation logic (highest-priority match wins):
  - Any overdue schedule → **Overdue**.
  - Any schedule due soon (within configurable thresholds, default 30 days or 500 mi) → **Upcoming Maintenance**.
  - No activity/records within a configurable inactivity window (default 365 days) → **Inactive**.
  - Otherwise → **Healthy**.

### 4.9 Activity & Auditing
- FR-ACT-1: The platform captures operational actions across domains. Minimum event types: vehicle
  added, maintenance completed, fuel log added, member invited, schedule marked overdue.
- FR-ACT-2: Two views: a fleet-level activity feed and a per-vehicle activity timeline.
- FR-ACT-3: Activity records are append-only and retain the actor, timestamp, and affected entity.

### 4.10 Notifications
- FR-NOTIF-1: In-app notifications only. Types: upcoming maintenance reminders, overdue maintenance
  alerts, fleet activity alerts.
- FR-NOTIF-2: Notification settings are configurable **per user, per type** (enable/disable).
- FR-NOTIF-3: Notifications are generated by consuming domain events and by scheduled reminder jobs;
  generation must be idempotent (no duplicate notification for the same trigger).

### 4.11 Documents & Media
- FR-MEDIA-1: Supported uploads: images, PDFs, receipts, and general maintenance documentation.
- FR-MEDIA-2: Objects are stored in MinIO. Each object persists metadata (owner, fleet, content
  type, size, original filename, status).
- FR-MEDIA-3: Soft-delete with async cleanup jobs; secure access controls — access is fleet-scoped
  and authorized; downloads are served via short-lived presigned URLs (no public buckets).
- FR-MEDIA-4: Image objects get async-generated variants (at least a thumbnail and a display size).

## 5. API Surface

### 5.1 Conventions
- JSON:API-inspired resource documents (`type`, `id`, `attributes`, `relationships`, `meta`).
- JWT bearer authentication on all non-public endpoints (`Authorization: Bearer <jwt>`).
- **Fleet-scoped authorization**: every resource access is checked against the caller's membership
  and role in the owning fleet. Cross-fleet access returns 404 (not 403) to avoid leaking existence.
- Structured error responses: `{ "errors": [ { "status", "code", "title", "detail", "source" } ] }`.
- Collection endpoints support pagination (`page[number]`/`page[size]` or cursor), filtering
  (`filter[...]`), and sorting (`sort=`). Responses include pagination `meta`/`links`.
- A single-origin **API gateway** routes by path prefix to each service (see §7):
  `/api/auth/*`, `/api/fleet/*` (and core domain), `/api/media/*`, `/api/notifications/*`.

### 5.2 Representative endpoints (grouped by owning service)

auth-service (`/api/auth`)
- `GET  /auth/login/google` → redirect to Google OIDC; `GET /auth/callback` → mint JWT + refresh.
- `POST /auth/refresh` → new access token. `POST /auth/logout`.
- `GET  /auth/me` → current user profile + active fleet/role.

fleet-service (`/api/fleet`, owns all core domain)
- Fleets: `POST /fleets`, `GET /fleets/{id}`, `PATCH /fleets/{id}` (rename, owner-only).
- Membership: `GET /fleets/{id}/members`, `DELETE /fleets/{id}/members/{userId}` (owner-only).
- Invites: `POST /fleets/{id}/invites`, `GET /fleets/{id}/invites`, `DELETE /invites/{id}`,
  `POST /invites/{token}/accept` (same-email enforced).
- Vehicles: `GET/POST /fleets/{id}/vehicles`, `GET/PATCH/DELETE /vehicles/{id}` (DELETE = soft),
  `POST /vehicles/{id}/restore` (owner-only), `PUT /vehicles/{id}/primary-image`.
- Mileage: `GET/POST /vehicles/{id}/mileage` (chronological; supports graphing query params).
- Maintenance: `GET /maintenance-categories`; `GET/POST /vehicles/{id}/maintenance-records`,
  `GET/PATCH/DELETE /maintenance-records/{id}`; `GET/POST /vehicles/{id}/maintenance-schedules`,
  `GET/PATCH/DELETE /maintenance-schedules/{id}`, `POST /maintenance-schedules/{id}/complete`.
- Maintenance queue: `GET /fleets/{id}/maintenance/upcoming`, `GET /fleets/{id}/maintenance/overdue`.
- Fuel: `GET/POST /vehicles/{id}/fuel-logs`, `GET/PATCH/DELETE /fuel-logs/{id}`.
- Dashboard: `GET/PUT /fleets/{id}/dashboard` (per-user widget layout).
- Activity: `GET /fleets/{id}/activity`, `GET /vehicles/{id}/activity`.

media-service (`/api/media`)
- `POST /media` (multipart or presigned-upload init), `GET /media/{id}` (metadata),
  `GET /media/{id}/download` (short-lived presigned URL), `DELETE /media/{id}` (soft).

notification-service (`/api/notifications`)
- `GET /notifications` (current user, filterable by read/type), `POST /notifications/{id}/read`,
  `POST /notifications/read-all`.
- `GET/PUT /notification-preferences` (per-user, per-type toggles).

Error cases (representative): 401 (missing/invalid JWT), 404 (resource not in caller's fleet),
403 (role lacks permission, e.g. viewer attempting a write, member attempting owner-only action),
409 (invite already accepted/expired; same-email mismatch), 410 (purged soft-deleted entity),
422 (validation).

## 6. Data Model

Per the §7 decision, **each service owns its own database/schema; no cross-service joins** — links
across service boundaries are by id reference, resolved at the application layer. All tables use
GORM-managed automated migrations, soft-delete (`deleted_at`) where noted, and indexes on
fleet-scoping foreign keys.

### auth-service
- `users`: id (uuid), google_sub (unique), email (unique), display_name, avatar_url,
  created_at, updated_at, last_login_at.
- `refresh_tokens` (or equivalent): id, user_id, token_hash, expires_at, revoked_at, created_at.

### fleet-service (owns the core domain)
- `fleets`: id, name, created_by_user_id, created_at, updated_at, deleted_at.
- `fleet_memberships`: id, fleet_id, user_id, role (`owner`|`member`|`viewer`),
  status (`active`|`invited`), created_at, updated_at. Unique (fleet_id, user_id). Many-to-many
  capable; MVP enforces a single *active* membership per user at the application layer.
- `fleet_invites`: id, fleet_id, email, role, token (unique), expires_at, accepted_at,
  invited_by_user_id, created_at.
- `vehicles`: id, fleet_id (idx), nickname, make, model, trim, year, vin, current_mileage,
  primary_image_media_id (ref media-service), notes, created_at, updated_at, deleted_at,
  purge_after (deleted_at + 5d).
- `vehicle_media`: id, vehicle_id (idx), media_id (ref media-service), is_primary, sort_order,
  created_at, deleted_at.
- `mileage_records`: id, vehicle_id (idx), mileage, recorded_at, source (`fuel`|`maintenance`|`manual`),
  source_ref_id, created_by_user_id, created_at. (Append-only.)
- `maintenance_categories`: id, name, description, system_defined (bool). Seeded.
- `maintenance_records`: id, vehicle_id (idx), category_id, date, mileage, cost, provider, notes,
  created_by_user_id, created_at, updated_at, deleted_at.
- `maintenance_record_documents`: id, maintenance_record_id (idx), media_id (ref media-service).
- `maintenance_schedules`: id, vehicle_id (idx), category_id, recurrence_type (`time`|`mileage`|`hybrid`),
  interval_months, interval_miles, last_completed_date, last_completed_mileage, next_due_date,
  next_due_mileage, severity (`informational`|`recommended`|`urgent`), status (`ok`|`upcoming`|`overdue`),
  created_at, updated_at, deleted_at.
- `fuel_logs`: id, vehicle_id (idx), date, mileage, gallons, total_cost, price_per_gallon,
  created_by_user_id, created_at, updated_at, deleted_at.
- `dashboards`: id, fleet_id, user_id (idx), created_at, updated_at. Unique (fleet_id, user_id).
- `dashboard_widgets`: id, dashboard_id (idx), type, position_x, position_y, width, height,
  config (jsonb), created_at, updated_at.
- `activity_events`: id, fleet_id (idx), vehicle_id (nullable, idx), actor_user_id, type,
  payload (jsonb), created_at. (Append-only.)

### media-service
- `media_objects`: id, fleet_id (idx), uploaded_by_user_id, bucket, object_key, content_type,
  size, original_filename, status (`uploaded`|`processing`|`ready`), created_at, deleted_at,
  purge_after.
- `media_variants`: id, media_object_id (idx), variant (`thumbnail`|`display`), object_key, width,
  height, content_type, created_at.

### notification-service
- `notifications`: id, user_id (idx), fleet_id, type, title, body, related_entity_type,
  related_entity_id, dedupe_key (unique per trigger), read_at, created_at.
- `notification_preferences`: id, user_id (idx), type, in_app_enabled (bool), created_at, updated_at.
  Unique (user_id, type).

Migration notes: seed `maintenance_categories` in a migration. All `purge_after`-bearing tables are
swept by async cleanup jobs. Referential integrity is enforced **within** a service's database only.

## 7. Service Impact

This is a greenfield, microservice-based build. The MVP creates the following components. Four
cross-cutting architectural decisions are **fixed by this PRD** (resolved during speccing):

> **D1 — Domain ownership:** `fleet-service` owns *all* core domain (fleets, memberships, invites,
> vehicles, vehicle-media metadata, mileage, maintenance categories/records/schedules, fuel,
> dashboards, vehicle-status derivation, and activity). `auth`, `media`, and `notification` stay narrow.
> **D2 — Database topology:** a single PostgreSQL instance with an isolated database/schema per
> service; no cross-service joins.
> **D3 — Auth model:** `auth-service` verifies Google OIDC and mints first-party JWTs; all services
> validate those JWTs.
> **D4 — Frontend↔backend routing:** a single-origin API gateway / reverse proxy routes by path
> prefix to each service (centralizing TLS, CORS, and JWT validation at the edge where practical).

Components:
- **auth-service** (Go): Google OIDC verification, user provisioning, JWT minting + refresh,
  `/auth/me`. Owns `users`, `refresh_tokens`.
- **fleet-service** (Go): the core domain per D1. Produces domain events; writes activity feed;
  runs recurrence recomputation and soft-delete cleanup jobs for its entities.
- **media-service** (Go): MinIO-backed uploads, metadata, presigned downloads, async thumbnail/
  resize processing, soft-delete cleanup of media objects.
- **notification-service** (Go): consumes domain events + scheduled reminders → in-app notifications;
  per-user preferences.
- **web** (React + TypeScript + ShadCN + Tailwind + TanStack Query): the SPA, served behind the gateway.
- **API gateway / reverse proxy** (per D4): single origin for the web app.
- **Shared packages**: `shared-go`, `shared-ts`, `dto-go` (transport DTOs), `ui-components`.
- **Infrastructure**: PostgreSQL, MinIO, Kafka (event bus), all via docker-compose locally and
  Kubernetes (k3s-compatible) manifests for deploy.

Event-driven workflows (Kafka). Representative events produced by `fleet-service`:
`vehicle.created`, `maintenance.completed`, `fuel.logged`, `schedule.overdue`, `member.invited`.
Consumers: `notification-service` (alerts/reminders), `media-service` (image-processing triggers on
media events). Event consumption must be **idempotent** with retry support.

## 8. Non-Functional Requirements

Security
- All endpoints (except the OAuth handshake) require a valid first-party JWT; fleet-scoped
  authorization on every resource; cross-fleet reads return 404.
- Secrets via environment/K8s Secrets, never committed; Gitleaks scanning fails builds on detected
  secrets across source, YAML, Dockerfiles, workflow files, and env config.
- Object storage is private; downloads only via short-lived presigned URLs.

Observability
- Every backend service emits structured logs (logrus or zerolog), OpenTelemetry traces with
  correlation IDs propagated across service and event boundaries, and exposes health and metrics
  endpoints.

Background processing
- Jobs (reminder generation, async soft-delete cleanup, image/thumbnail processing, recurrence
  recalculation) must be idempotent, retry-safe, and observable.

Reliability & deploy
- Multi-stage Docker builds; containers run as non-root; health endpoints exposed; configuration
  via environment.
- Kubernetes: k3s-compatible manifests (raw YAML or Kustomize), ConfigMap/Secret separation,
  resource requests/limits, readiness + liveness probes, rolling deploys.

Performance (MVP targets — refine in design)
- Interactive API reads (dashboard, lists) target P95 < 300 ms under typical single-household load.
- Image processing and reminder generation are async and must not block request paths.

Data integrity
- Referential integrity within each service DB; automated GORM migrations; soft-delete with
  enforced recovery windows; mileage and activity records are append-only.

CI/CD & dependency hygiene
- GitHub Actions PR workflow: build validation, TypeScript checks, Go tests, container-build
  validation, Gitleaks, formatting validation.
- GitHub Actions main workflow: full builds, Docker image publishing to GHCR, version tagging,
  deployment-artifact generation, vulnerability scanning.
- Renovate: monorepo-aware, grouped compatible updates, minimum release age 7–14 days, separated
  major upgrades, automerge disabled initially; ecosystems: Go modules, npm, Docker, GitHub Actions.

## 9. Open Questions

These do not block the MVP contract but should be resolved in design/planning:
- Monorepo tooling: Go workspaces (`go.work`) + npm workspaces vs. a dedicated monorepo tool
  (Turborepo/Nx). Repo layout (`/apps`, `/packages`, `/deploy`, `/scripts`, `/docs`) is fixed; the
  tool is not.
- API gateway choice (Traefik vs. Nginx vs. other) and how much auth it terminates at the edge vs.
  per-service JWT validation.
- JWT signing strategy (symmetric vs. asymmetric/JWKS) and access/refresh token lifetimes.
- Dashboard "Spend by vehicle" and "Mileage trends" widgets: exact aggregation windows and whether
  precomputed or computed on read.
- Notification reminder cadence/thresholds and the exact "due soon" thresholds feeding vehicle status.
- Whether `media-service` performs image processing inline on event vs. a dedicated worker.
- Pagination style (page-number vs. cursor) standardized across services.

## 10. Acceptance Criteria

The MVP is "done" when, end-to-end and verified:

Auth & fleet
- [ ] A user can sign in with Google; an account is auto-provisioned; the app issues a first-party JWT and refreshes it.
- [ ] A user can create a fleet with a name during onboarding and lands on a dashboard.
- [ ] An owner can invite by email (with role + expiration); only the matching-email user can accept; expired/used invites are rejected.
- [ ] Owners can rename the fleet and remove members; viewers cannot perform writes; members cannot perform owner-only actions.

Vehicles, mileage, media
- [ ] Members can add/edit/soft-delete vehicles with all specified fields; owners can restore within 5 days; cleanup purges afterward.
- [ ] Multiple images per vehicle upload with a selectable primary; thumbnails/display variants are generated asynchronously.
- [ ] Mileage is recorded as history from fuel, maintenance, and manual sources; latest mileage auto-fills forms and powers a trend graph.

Maintenance & fuel
- [ ] Members can log maintenance (date, mileage, cost, provider, notes, category) and attach receipts/documents.
- [ ] Recurring maintenance supports time, mileage, and hybrid rules; the upcoming and overdue queues populate with correct severity.
- [ ] Completing a scheduled item creates a pre-populated maintenance record and recomputes the next due point.
- [ ] Fuel logs capture all fields and create a mileage record.

Dashboard, status, activity, notifications
- [ ] Users can add/remove/reorder/resize widgets from the predefined catalog; layout persists per user.
- [ ] Vehicle status derives correctly across Healthy / Upcoming / Overdue / Inactive per the defined logic.
- [ ] Fleet activity feed and per-vehicle timeline capture the specified event types.
- [ ] In-app notifications fire for upcoming/overdue/activity, are de-duplicated, and respect per-user per-type preferences.

Platform & ops
- [ ] `docker-compose up` brings up web + all services + PostgreSQL + MinIO + Kafka locally.
- [ ] All services: multi-stage Docker builds, non-root, health + metrics endpoints, structured logs, OTel traces with correlation IDs.
- [ ] Kubernetes (k3s) manifests deploy all services with probes, resource limits, and ConfigMap/Secret separation.
- [ ] CI: PR workflow (build, TS checks, Go tests, container build, Gitleaks, formatting) and main workflow (build, GHCR publish, tagging, vuln scan) are green.
- [ ] Renovate is configured per policy; `go test -race ./...`, `go vet ./...`, `go build ./...`, `npm run build`, and `npm test` are clean.
