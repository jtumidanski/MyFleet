FROM golang:1.27-alpine AS build
WORKDIR /src

# Module manifests first, so the `go mod download` layer below is keyed on
# dependency changes rather than on source churn. This saves the download only
# (~8s): `go mod download` populates GOMODCACHE, not GOCACHE, so the compile
# below still rebuilds every dependency whenever a source COPY is invalidated.
# go.work lists every service module, so all of their go.mod/go.sum must be
# present for the workspace to load even when only one service is built.
# The go.su[m] bracket glob makes that file optional: packages/dto-go has no
# external dependencies today and therefore no go.sum, but if it gains one the
# sum lands in this layer instead of being silently ignored.
COPY go.work go.work.sum ./
COPY packages/dto-go/go.mod packages/dto-go/go.su[m] ./packages/dto-go/
COPY packages/shared-go/go.mod packages/shared-go/go.sum ./packages/shared-go/
COPY apps/auth-service/go.mod apps/auth-service/go.sum ./apps/auth-service/
COPY apps/fleet-service/go.mod apps/fleet-service/go.sum ./apps/fleet-service/
COPY apps/media-service/go.mod apps/media-service/go.sum ./apps/media-service/
COPY apps/notification-service/go.mod apps/notification-service/go.sum ./apps/notification-service/
RUN go mod download

# Sources. Only the two Go workspace packages — copying all of packages/ would
# drag in shared-ts and ui-components, so a frontend-only commit would
# invalidate the compile layer of all four Go images. The other services'
# sources are not needed (no service imports another).
COPY packages/dto-go ./packages/dto-go
COPY packages/shared-go ./packages/shared-go
COPY apps/notification-service ./apps/notification-service
RUN cd apps/notification-service && go build -o /out/notification-service ./cmd

FROM alpine:3.24
RUN adduser -D -u 10001 app
USER app
COPY --from=build /out/notification-service /notification-service
EXPOSE 8080
ENTRYPOINT ["/notification-service"]
