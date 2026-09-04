---
description: Generate or update documentation for one MyFleet service — dispatches the service-documentation agent
argument-hint: Service name or path (e.g., "fleet-service" or "apps/fleet-service")
---

Dispatch the `service-documentation` agent against: **$ARGUMENTS**.

The agent treats code as the single source of truth, follows `DOCS.md`, and operates only within the target service directory under `apps/`. It outputs only updated doc files — no commentary, no analysis.
