# syntax=docker/dockerfile:1
# Production image for @myfleet/web. Build context = repo ROOT (the npm
# workspace lives there). Only the JS/TS workspace is needed — no go.work.

# --- Stage 1: build the SPA -------------------------------------------------
FROM node:20-alpine AS build
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
FROM nginx:alpine
COPY apps/web/nginx.conf /etc/nginx/conf.d/default.conf
COPY --from=build /app/apps/web/dist /usr/share/nginx/html
EXPOSE 80
