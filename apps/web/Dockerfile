# syntax=docker/dockerfile:1
# Production image for @myfleet/web. Build context = repo ROOT (the npm
# workspace lives there). Only the JS/TS workspace is needed — no go.work.

# --- Stage 1: build the SPA -------------------------------------------------
FROM node:24-alpine AS build
WORKDIR /app

# Copy the workspace manifests first so `npm ci` is cached independently of
# source churn. The root package.json declares the apps/web,
# packages/shared-ts and packages/ui-components workspaces.
COPY package.json package-lock.json ./
COPY apps/web/package.json ./apps/web/package.json
COPY packages/shared-ts/package.json ./packages/shared-ts/package.json
COPY packages/ui-components/package.json ./packages/ui-components/package.json

RUN npm ci

# Now the sources for the web app and the two TS workspaces it imports.
COPY packages/shared-ts ./packages/shared-ts
COPY packages/ui-components ./packages/ui-components
COPY apps/web ./apps/web

RUN npm run -w @myfleet/web build

# --- Stage 2: static server -------------------------------------------------
# Unprivileged nginx (UID 101, binds :8080) so the container runs with the same
# hardened securityContext as the Go services: runAsNonRoot + readOnlyRootFilesystem.
FROM nginxinc/nginx-unprivileged:alpine
# Same apk lag as the Go services' alpine base: the tag ships openssl 3.5.7-r0
# while the CVE-2026-14456 fix (3.5.8-r0) is already in v3.24/main. This image
# defaults to USER 101, so drop to root for the upgrade and go straight back —
# the runtime user is unchanged.
USER root
RUN apk upgrade --no-cache
USER 101
COPY apps/web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/apps/web/dist /usr/share/nginx/html
EXPOSE 8080
