# Household Vehicle Management Platform - MVP PRD

## Document Information

- **Version:** 1.0
- **Type:** MVP Product Requirements Document
- **Status:** Draft

---

# 1. Product Overview

This project is a cloud-hosted household vehicle management platform focused on collaborative maintenance tracking, operational visibility, recurring maintenance scheduling, and historical record retention for household-owned vehicles.

The platform is intended to support multiple independent households (“fleets”), each consisting of one or more users collaborating on a shared collection of vehicles.

The product philosophy emphasizes:

- Household-first workflows
- Collaborative ownership visibility
- Structured maintenance history
- Calm and productive user experience
- Dashboard-driven operational visibility
- Future extensibility toward intelligent maintenance prediction and automation

The MVP intentionally prioritizes practical utility and data quality over AI-first workflows.

---

# 2. Goals

## Primary Goals

- Provide a collaborative household vehicle management platform
- Track vehicle maintenance history and recurring maintenance requirements
- Support shared household ownership workflows
- Provide dashboard visibility into upcoming and overdue maintenance
- Preserve historical records, documents, and receipts
- Establish structured data foundations for future intelligent workflows

## Secondary Goals

- Support enthusiast-oriented organization and history retention
- Provide operational audit visibility
- Create extensible event-driven architecture foundations

---

# 3. Explicit Non-Goals

The following are intentionally out of scope for MVP:

- Native mobile applications
- OCR receipt ingestion
- AI maintenance extraction
- Predictive maintenance AI
- Mechanic marketplace integrations
- OBD/vehicle telemetry integrations
- Advanced financial analytics
- VIN decoding integrations
- Real-time collaborative editing
- Social/community features

---

# 4. Target Users

## Primary Users

- Households with multiple vehicles
- Families coordinating maintenance responsibilities
- Users seeking structured maintenance tracking and reminders

## Secondary Users

- Car enthusiasts managing multiple vehicles
- Project car owners
- Multi-driver households

---

# 5. Authentication & Identity Requirements

- Google OAuth must be the only authentication provider for MVP
- OIDC-compatible architecture must be used
- Users must authenticate using Google accounts
- User accounts are automatically provisioned on first login
- JWT-based authentication is required
- Fleet invites require matching authenticated email address
- Users may only actively belong to one fleet during MVP, but the data model must support many-to-many membership relationships

---

# 6. Fleet & Membership Requirements

## Fleet Onboarding

- Users create a fleet during onboarding
- Fleet requires a user-defined name
- System may suggest default names

## Roles

### Fleet Owner

Can:
- Invite users
- Remove users
- Rename fleet
- Manage vehicles
- Restore deleted vehicles

### Fleet Member

Can:
- Add/edit maintenance records
- Add fuel logs
- Upload documents
- Manage schedules

### Viewer (Optional)

Read-only access.

## Invite Workflow

- Email-based invitations
- Invite expiration support
- Same-email enforcement required

---

# 7. Vehicle Management Requirements

## Vehicle CRUD

- Add vehicle
- Edit vehicle
- Soft delete vehicle
- Restore deleted vehicle

## Vehicle Fields

- Nickname/display name (optional)
- Make
- Model
- Trim (optional)
- Year
- VIN (optional)
- Current mileage
- Primary image
- Notes

## Vehicle Media

- Multiple images per vehicle
- Primary image selection
- Async thumbnail generation
- Image resizing/compression

## Soft Delete Behavior

- Vehicles remain recoverable for 5 days
- Async cleanup jobs permanently purge deleted entities

---

# 8. Mileage Tracking Requirements

## Mileage History

Mileage must be tracked through dedicated mileage history records.

Mileage updates may originate from:
- Fuel logs
- Maintenance records
- Manual mileage updates

Mileage history must:
- Preserve chronological history
- Support graphing
- Support projections
- Support audit visibility

Latest mileage should auto-populate relevant forms where appropriate.

---

# 9. Maintenance System Requirements

## Maintenance Categories

- System-defined maintenance categories
- Structured and standardized categories

## Maintenance History

Users must be able to:
- Add maintenance records
- Edit maintenance records
- Associate maintenance records with vehicles
- Attach documents and receipts

Maintenance records must support:
- Date
- Mileage
- Cost
- Provider/shop
- Notes

## Recurring Maintenance

The platform must support:
- Time-based recurrence
- Mileage-based recurrence
- Hybrid recurrence rules

### Examples

- Oil change every 5,000 miles OR 12 months
- Tire rotation every 7,500 miles

## Maintenance Scheduling

Support:
- Upcoming maintenance queue
- Overdue maintenance tracking
- Maintenance severity levels

### Severity Levels

- Informational
- Recommended
- Urgent

## Completion Workflow

Scheduled maintenance must transition cleanly into historical maintenance records.

Completion flow should pre-populate known values.

---

# 10. Fuel Tracking Requirements

## Fuel Logging

Fuel entries must support:
- Date
- Mileage
- Gallons
- Total cost
- Price per gallon

Fuel tracking must:
- Update mileage history
- Support future MPG calculations
- Support cost aggregation

---

# 11. Dashboard System Requirements

## Dashboard Capabilities

Users must be able to:
- Add widgets
- Remove widgets
- Reorder widgets
- Resize widgets

## Dashboard Widgets

Initial widgets:
- Fleet overview
- Vehicle status cards
- Upcoming maintenance
- Overdue maintenance
- Recent activity feed
- Spend by vehicle
- Mileage trends

## Dashboard Philosophy

- Predefined widgets only
- No arbitrary BI/query tooling in MVP

---

# 12. Activity & Auditing Requirements

The platform must capture operational actions across the platform.

## Example Events

- Vehicle added
- Maintenance completed
- Fuel log added
- Member invited
- Schedule marked overdue

## Activity Views

- Fleet-level activity feed
- Vehicle-level activity timeline

## Auditing Goals

- Collaboration visibility
- Trust and transparency
- Historical traceability

---

# 13. Notification Requirements

## Notification Channels

MVP supports:
- In-app notifications only

## Notification Types

- Upcoming maintenance reminders
- Overdue maintenance alerts
- Fleet activity alerts

## Notification Preferences

Notification settings must be configurable per user.

---

# 14. Documents & Media Requirements

## Supported Uploads

- Images
- PDFs
- Receipts
- Maintenance documentation

## Storage

- MinIO object storage

## Requirements

- Soft delete support
- Async cleanup jobs
- Metadata persistence
- Secure access controls

---

# 15. Vehicle Status System

Vehicle status must be derived automatically.

## Supported Statuses

- Healthy
- Upcoming Maintenance
- Overdue
- Inactive

## Example Derived Logic

- Overdue maintenance => Overdue
- Maintenance due soon => Upcoming Maintenance
- No issues => Healthy

---

# 16. UI/UX Requirements

## Frontend Stack

- React
- TypeScript
- ShadCN UI
- Tailwind CSS
- TanStack Query

## UX Philosophy

- Calm and productive
- Operationally focused
- Desktop-first
- Responsive/mobile-compatible

## Navigation Model

```text
Fleet
  -> Vehicle
      -> Dashboard
      -> Maintenance
      -> Fuel
      -> Documents
      -> Timeline
      -> Settings
```

The product should avoid enthusiast-forum aesthetics and instead emphasize clarity and operational organization.

---

# 17. Technical Architecture

## Architecture Style

- Microservice-based architecture

## Primary Services

- auth-service
- fleet-service
- media-service
- notification-service

## Infrastructure

- PostgreSQL
- MinIO
- Kafka/event bus
- Kubernetes deployment support

## Internal Communication

- REST APIs
- Internal event-driven workflows

### Example Events

- vehicle.created
- maintenance.completed
- fuel.logged
- schedule.overdue

---

# 18. API Requirements

## API Style

- JSON:API-inspired conventions

## Requirements

- Fleet-scoped authorization
- Structured error responses
- Pagination support
- Filtering support
- Sorting support

## Authentication

- JWT bearer authentication

---

# 19. Background Processing Requirements

## Background Jobs

- Reminder generation
- Async cleanup
- Image processing
- Thumbnail generation
- Maintenance recurrence recalculation

## Processing Requirements

- Idempotent processing
- Retry support
- Observability support

---

# 20. Database Requirements

## Primary Database

- PostgreSQL

## ORM

- Gorm

## Requirements

- Automated migrations
- Soft delete support
- Referential integrity
- Indexing for fleet-scoped queries

---

# 21. CI/CD Requirements

## GitHub Actions - Pull Request Workflow

Responsibilities:
- Build validation
- TypeScript checks
- Go tests
- Container build validation
- Gitleaks scanning
- Formatting validation

## GitHub Actions - Main Workflow

Responsibilities:
- Full builds
- Docker image publishing
- Version tagging
- Deployment artifact generation
- Vulnerability scanning

## Container Registry

- GitHub Container Registry (GHCR)

---

# 22. Docker Requirements

All services must:
- Use multi-stage Docker builds
- Run as non-root users
- Expose health endpoints
- Support environment-based configuration

## Local Development

A complete docker-compose environment must be provided.

## Docker Compose Stack

- Frontend
- Backend services
- PostgreSQL
- MinIO
- Kafka

---

# 23. Kubernetes Requirements

## Deployment Requirements

- Kubernetes manifests included
- k3s-compatible
- ConfigMaps and Secrets separation
- Resource requests/limits
- Readiness and liveness probes
- Rolling deployment support

## Deployment Artifacts

- Raw YAML or Kustomize

---

# 24. Renovate Requirements

Renovate must:
- Support monorepo dependency management
- Group compatible dependency updates
- Delay merge eligibility using minimum release age
- Separate major version upgrades

## Supported Ecosystems

- Go modules
- npm
- Docker
- GitHub Actions

## Policies

- Minimum release age: 7-14 days
- Automerge disabled initially

---

# 25. Gitleaks Requirements

Gitleaks scanning must:
- Execute during PR workflows
- Execute during main branch workflows
- Fail builds on detected secrets

## Coverage

- Source code
- YAML
- Dockerfiles
- Workflow files
- Environment configuration

---

# 26. Observability Requirements

All backend services must support:
- Structured logging
- OpenTelemetry tracing
- Correlation IDs
- Health endpoints
- Metrics endpoints

## Preferred Tooling

- logrus or zerolog
- OTLP exporters

---

# 27. Repository Structure

```text
/apps
  /web
  /auth-service
  /fleet-service
  /media-service
  /notification-service

/packages
  /shared-go
  /shared-ts
  /dto-go
  /ui-components

/deploy
  /docker-compose
  /kubernetes

/scripts
/docs
```

---

# 28. Future Roadmap

Potential future roadmap items:
- OCR receipt ingestion
- VIN decoding integrations
- Predictive maintenance
- AI-assisted maintenance extraction
- Mobile applications
- Email notifications
- Push notifications
- Maintenance forecasting
- External service integrations
- Cost analytics
- Resale/export packages
