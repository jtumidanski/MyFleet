FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.work go.work.sum ./
COPY packages ./packages
COPY apps/notification-service ./apps/notification-service
RUN cd apps/notification-service && go build -o /out/notification-service ./cmd

FROM alpine:3.20
RUN adduser -D -u 10001 app
USER app
COPY --from=build /out/notification-service /notification-service
EXPOSE 8080
ENTRYPOINT ["/notification-service"]
