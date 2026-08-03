---
name: frontend-dev-guidelines
description: Skill for creating and modifying the MyFleet UI frontend using React, TypeScript, Vite, shadcn/ui, TanStack React Query, react-hook-form with Zod validation, and Tailwind CSS.
---


# Frontend Development Skill

## Purpose
Provide a composable entry point that activates when working on the MyFleet UI. This skill aligns development and AI generation with the established frontend architecture patterns and conventions.

## When to Use
Activate when working on:
- Any file inside `apps/web/`
- React components (`.tsx` files in `components/` or `pages/`)
- Custom hooks (`lib/hooks/` or `lib/hooks/api/`)
- API service layer (`services/api/`)
- Zod validation schemas (`lib/schemas/`)
- TypeScript type definitions (`types/`)
- React Query integration and cache management
- Form components using react-hook-form
- Styling with Tailwind CSS and shadcn/ui
- Testing with Vitest and React Testing Library
- The shared frontend packages `packages/shared-ts` and `packages/ui-components`

---

## Quick Start Checklist
- [ ] **Component** follows presentational/container split (ui/ vs features/)
- [ ] **Types** defined with JSON:API structure (`id` + `attributes`)
- [ ] **Service** extends `BaseService` — every concrete service does; there is no second pattern
- [ ] **Hook** uses query key factory with hierarchical keys (`as const`)
- [ ] **Form** uses `react-hook-form` with `zodResolver` and Zod schema
- [ ] **Validation schema** defined in `lib/schemas/` with inferred types
- [ ] **Loading state** uses skeleton components, not spinners (except submit buttons)
- [ ] **Error handling** uses `createErrorFromUnknown()` (from `@myfleet/shared-ts`) and toast notifications
- [ ] **Styling** uses Tailwind utility classes with `cn()` helper
- [ ] **Tests** written with Vitest + React Testing Library, as siblings of the file under test
- [ ] **Test execution** verified before claiming completion

---

## Standard Implementation Workflow

**MANDATORY:** Follow this workflow for ALL code changes.

### Implementation Steps

When modifying any UI code:

1. **Read existing code** before making changes — understand the current patterns in use
2. **Implement changes** following the patterns documented in this skill
3. **Update types** if API contracts changed (`types/models/`, and the envelope types in `@myfleet/shared-ts`)
4. **Update service layer** if new API endpoints are needed
5. **Update query hooks** if data fetching patterns changed
6. **Run tests BEFORE claiming completion**:
   ```bash
   make fe-test
   ```
7. **Fix any failures** — Do NOT skip or ignore test failures
8. **Verify build**:
   ```bash
   make fe-build
   ```
9. **Report test results** with actual command output, not assumptions

### Critical Rules

- **Never skip test execution** — Running tests is mandatory, not optional
- **Never assume tests will pass** — Always verify with actual execution
- **Never mutate state directly** — Use immutable update patterns
- **Never use `any` type** — TypeScript strict mode is enabled; use proper types
- **Never inline Zod schemas in components** — Define schemas in `lib/schemas/`
- **Always use `cn()` for conditional classes** — Never manual string concatenation
- **Always use skeleton components for loading** — Not raw spinners in content areas
- **Always use toast for user feedback** — `toast.success()`, `toast.error()` via sonner
- **Always verify test output** before marking work complete

### When Tests Fail

If `make fe-test` reports failures:

1. **Read the error message completely** — understand what broke
2. **Check the `vi.mock` specifiers** — they are real relative paths, so a moved or renamed service silently stops being stubbed. This is the most common cause of a component test failing after a refactor.
3. **Check the component is rendered through `renderWithProviders`** — anything using a React Query hook or a `<Link>` needs the QueryClient and router it supplies (`src/test/renderWithProviders.tsx`)
4. **Re-run tests** — verify the fix didn't break others
5. **Do not proceed** until all tests pass

See [Testing Guide](resources/testing-guide.md) for comprehensive testing guidelines.

---

## Key Principles
1. **JSON:API Compliance** — All models use `{ id, attributes }` structure matching backend services.
2. **Type Safety** — TypeScript strict mode with no `any`; use type guards for runtime checks.
3. **Server State via React Query** — All server data managed through TanStack React Query hooks.
4. **Composition over Configuration** — shadcn/ui composable primitives, not monolithic components.
5. **Immutable Updates** — Spread operators for state updates; never mutate props or state directly.

---

## File Responsibilities

| Location | Primary Responsibility |
|----------|------------------------|
| `App.tsx` | The route tree (`AppRoutes`) and the router that hosts it |
| `main.tsx` | Mounts `<AppProviders><App /></AppProviders>` and latches runtime config |
| `pages/*.tsx` | Route pages — call hooks, own dialog state and permission decisions |
| `pages/admin/` | Platform-admin console pages |
| `components/ui/` | shadcn/ui primitives — button, dialog, input, sidebar |
| `components/features/` | Feature components, grouped by resource, with `dialogs/` beneath |
| `components/frame/` | Shell furniture shared by both layouts — header, nav, profile menu, breadcrumbs |
| `components/admin/` | Admin-console layout and guard |
| `components/providers/` | `AppProviders` — the provider stack |
| `lib/api/client.ts` | Constructs the singleton `ApiClient` from `@myfleet/shared-ts` and wires it to the auth contract (21 lines; it is not itself an HTTP client) |
| `lib/api/refresh.ts` · `token.ts` | Token storage and the refresh round-trip the client delegates to |
| `lib/hooks/api/` | React Query hooks — key factories, queries, mutations, invalidation |
| `lib/schemas/` | Zod schemas with inferred form-input types |
| `lib/utils.ts` | `cn()` classname utility |
| `services/api/` | `BaseService` plus one PascalCase service per resource, each exporting a singleton |
| `types/models/` | Domain model interfaces (`JsonApiResource<A>` + attribute types) |
| `context/` | React context definitions — `AuthContext`, `ThemeContext` |
| `test/` | `renderWithProviders` and the executable convention tests |
| `packages/shared-ts` | `ApiClient`, JSON:API envelope types, `ApiError`, `createErrorFromUnknown` |
| `packages/ui-components` | Cross-app presentational components (`StatusBadge`) and formatters |

There is no `components/common/`, no `lib/api/errors.ts`, no `types/api/`, no `lib/breadcrumbs/` and no `services/api/index.ts` barrel. <!-- ALLOW-VOCAB:G-21 -->

---

## Navigation Guide

| Topic | Reference |
|-------|-----------|
| Architecture Overview | [resources/architecture-overview.md](resources/architecture-overview.md) |
| Service Layer Patterns | [resources/patterns-service-layer.md](resources/patterns-service-layer.md) |
| React Query & Hooks | [resources/patterns-react-query.md](resources/patterns-react-query.md) |
| Component Patterns | [resources/patterns-components.md](resources/patterns-components.md) |
| Routing & Pages | [resources/patterns-routing.md](resources/patterns-routing.md) |
| Forms & Validation | [resources/patterns-forms-validation.md](resources/patterns-forms-validation.md) |
| Styling & Theming | [resources/patterns-styling.md](resources/patterns-styling.md) |
| API Client | [resources/patterns-api-client.md](resources/patterns-api-client.md) |
| Type System | [resources/patterns-types.md](resources/patterns-types.md) |
| Testing Guide | [resources/testing-guide.md](resources/testing-guide.md) |
| Anti-Patterns | [resources/anti-patterns.md](resources/anti-patterns.md) |
| AI Code Guidance | [resources/ai-guidance.md](resources/ai-guidance.md) |

---
