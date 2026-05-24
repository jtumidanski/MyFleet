FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.work go.work.sum ./
COPY packages ./packages
# go.work lists every service module, so all of their go.mod/go.sum must be
# present for the workspace to load even when only one service is built. The
# other services' sources are not needed (no service imports another).
COPY apps/auth-service/go.mod apps/auth-service/go.sum ./apps/auth-service/
COPY apps/fleet-service/go.mod apps/fleet-service/go.sum ./apps/fleet-service/
COPY apps/media-service/go.mod apps/media-service/go.sum ./apps/media-service/
COPY apps/notification-service/go.mod apps/notification-service/go.sum ./apps/notification-service/
COPY apps/notification-service ./apps/notification-service
RUN cd apps/notification-service && go build -o /out/notification-service ./cmd

FROM alpine:3.20
RUN adduser -D -u 10001 app
USER app
COPY --from=build /out/notification-service /notification-service
EXPOSE 8080
ENTRYPOINT ["/notification-service"]
